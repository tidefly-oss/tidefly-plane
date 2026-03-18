package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	applogger "github.com/tidefly-oss/tidefly-backend/internal/logger"
)

type RequestLoggerOptions struct {
	SlowThreshold time.Duration
}

type wrappedWriter struct {
	http.ResponseWriter
	status    int
	size      int64
	buf       bytes.Buffer
	committed bool
	capture   bool
}

func (w *wrappedWriter) WriteHeader(code int) {
	if w.committed {
		return
	}
	w.status = code
	w.committed = true
	w.capture = code >= 400
	w.ResponseWriter.WriteHeader(code)
}

func (w *wrappedWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *wrappedWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *wrappedWriter) Write(b []byte) (int, error) {
	if !w.committed {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.size += int64(n)
	if w.capture && w.buf.Len() < maxBodySize {
		rem := maxBodySize - w.buf.Len()
		if len(b) <= rem {
			w.buf.Write(b)
		} else {
			w.buf.Write(b[:rem])
		}
	}
	return n, err
}

func RequestLogger(log *applogger.Logger, opts ...RequestLoggerOptions) echo.MiddlewareFunc {
	var opt RequestLoggerOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			start := time.Now()
			req := c.Request()

			var reqBody []byte
			if req.Body != nil && isLoggableContentType(req.Header.Get("Content-Type")) {
				limited := io.LimitReader(req.Body, maxBodySize+1)
				if raw, err := io.ReadAll(limited); err == nil {
					reqBody = raw
					req.Body = io.NopCloser(bytes.NewReader(raw))
				}
			}

			wrapped := &wrappedWriter{ResponseWriter: c.Response(), status: http.StatusOK}
			c.SetResponse(wrapped)

			handlerErr := next(c)
			duration := time.Since(start)

			status := wrapped.status
			if echoResp, uErr := echo.UnwrapResponse(c.Response()); uErr == nil && echoResp.Committed {
				status = echoResp.Status
			}
			if handlerErr != nil {
				if he, ok := errors.AsType[*echo.HTTPError](handlerErr); ok {
					status = he.Code
				}
			}

			requestID := wrapped.Header().Get(echo.HeaderXRequestID)
			if requestID == "" {
				requestID = req.Header.Get(echo.HeaderXRequestID)
			}

			isSlow := opt.SlowThreshold > 0 && duration > opt.SlowThreshold
			isError := handlerErr != nil || status >= 400

			attrs := []any{
				slog.String("method", req.Method),
				slog.String("path", req.URL.Path),
				slog.String("query", req.URL.RawQuery),
				slog.Int("status", status),
				slog.Int64("latency_ms", duration.Milliseconds()),
				slog.String("ip", c.RealIP()),
				slog.String("request_id", requestID),
				slog.Int64("response_bytes", wrapped.size),
			}

			if v, ok := c.Get("user_id").(string); ok && v != "" {
				attrs = append(attrs, slog.String("user_id", v))
			}
			if isSlow {
				attrs = append(attrs, slog.Bool("slow_request", true))
			}
			if isError || isSlow {
				attrs = append(
					attrs,
					slog.String("content_type", req.Header.Get("Content-Type")),
					slog.String("user_agent", req.Header.Get("User-Agent")),
				)
				if len(reqBody) > 0 {
					attrs = append(
						attrs, slog.String(
							"request_body",
							truncate(redactBody(reqBody, req.Header.Get("Content-Type")), maxBodySize),
						),
					)
				}
			}
			if isError && wrapped.buf.Len() > 0 {
				attrs = append(
					attrs, slog.String(
						"response_body",
						truncate(redactBody(wrapped.buf.Bytes(), wrapped.Header().Get("Content-Type")), maxBodySize),
					),
				)
			}
			if handlerErr != nil {
				attrs = append(attrs, slog.String("error", handlerErr.Error()))
			}

			msg := req.Method + " " + req.URL.Path
			sl := log.Slog()
			if isError {
				sl.Error(msg, attrs...)
			} else {
				sl.Info(msg, attrs...)
			}

			return handlerErr
		}
	}
}

func redactBody(body []byte, contentType string) string {
	if !strings.Contains(contentType, "application/json") {
		return string(body)
	}
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return string(body)
	}
	redactMap(data)
	out, err := json.Marshal(data)
	if err != nil {
		return string(body)
	}
	return string(out)
}

func redactMap(m map[string]any) {
	for k, v := range m {
		if sensitiveFields[strings.ToLower(k)] {
			m[k] = "[REDACTED]"
			continue
		}
		switch val := v.(type) {
		case map[string]any:
			redactMap(val)
		case []any:
			for _, item := range val {
				if nested, ok := item.(map[string]any); ok {
					redactMap(nested)
				}
			}
		}
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + " …[truncated]"
}

func isLoggableContentType(ct string) bool {
	for _, t := range loggableContentTypes {
		if strings.Contains(ct, t) {
			return true
		}
	}
	return false
}
