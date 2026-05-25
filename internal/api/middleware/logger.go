package middleware

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
)

// ── Response Writer Wrapper ───────────────────────────────────────────────────

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

// Hijack implements http.Hijacker to support WebSocket upgrades.
func (w *wrappedWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("responsewriter does not implement http.Hijacker")
	}
	return h.Hijack()
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

// ── Middleware ────────────────────────────────────────────────────────────────

type RequestLoggerOptions struct {
	SlowThreshold time.Duration
}

func RequestLogger(log *applogger.Logger, opts ...RequestLoggerOptions) func(http.Handler) http.Handler {
	var opt RequestLoggerOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			var reqBody []byte
			if r.Body != nil && isLoggableContentType(r.Header.Get("Content-Type")) {
				limited := io.LimitReader(r.Body, maxBodySize+1)
				if raw, err := io.ReadAll(limited); err == nil {
					reqBody = raw
					r.Body = io.NopCloser(bytes.NewReader(raw))
				}
			}

			wrapped := &wrappedWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)
			status := wrapped.status
			requestID := chimiddleware.GetReqID(r.Context())

			isSlow := opt.SlowThreshold > 0 && duration > opt.SlowThreshold
			isError := status >= 400

			attrs := []any{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("query", r.URL.RawQuery),
				slog.Int("status", status),
				slog.Int64("latency_ms", duration.Milliseconds()),
				slog.String("ip", r.RemoteAddr),
				slog.String("request_id", requestID),
				slog.Int64("response_bytes", wrapped.size),
			}

			if claims := UserFromHumaCtx(r.Context()); claims != nil {
				attrs = append(attrs, slog.String("user_id", claims.UserID))
			}
			if isSlow {
				attrs = append(attrs, slog.Bool("slow_request", true))
			}
			if isError || isSlow {
				attrs = append(attrs,
					slog.String("content_type", r.Header.Get("Content-Type")),
					slog.String("user_agent", r.Header.Get("User-Agent")),
				)
				if len(reqBody) > 0 {
					attrs = append(attrs, slog.String("request_body",
						truncate(redactBody(reqBody, r.Header.Get("Content-Type")), maxBodySize)))
				}
			}
			if isError && wrapped.buf.Len() > 0 {
				attrs = append(attrs, slog.String("response_body",
					truncate(redactBody(wrapped.buf.Bytes(), wrapped.Header().Get("Content-Type")), maxBodySize)))
			}

			msg := r.Method + " " + r.URL.Path
			sl := log.Slog()
			if isError {
				sl.Error(msg, attrs...)
			} else {
				sl.Info(msg, attrs...)
			}
		})
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

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
