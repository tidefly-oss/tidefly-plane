package containers

// Logs und Stats bleiben auf rohem Echo wegen SSE-Streaming.

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
	dockerruntime "github.com/tidefly-oss/tidefly-backend/internal/services/runtime/docker"
)

func (h *Handler) Logs(c *echo.Context) error {
	id := c.Param("id")
	tail := c.QueryParam("tail")
	if tail == "" {
		tail = "100"
	}
	since := c.QueryParam("since")
	timestamps, _ := strconv.ParseBool(c.QueryParam("timestamps"))

	resp := c.Response()
	resp.Header().Set("Content-Type", "text/event-stream")
	resp.Header().Set("Cache-Control", "no-cache")
	resp.Header().Set("Connection", "keep-alive")
	resp.Header().Set("X-Accel-Buffering", "no")
	resp.WriteHeader(http.StatusOK)
	flusher, ok := resp.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}

	ctx := c.Request().Context()
	logs, err := h.runtime.ContainerLogs(
		ctx, id, runtime.LogOptions{
			Follow: true, Tail: tail, Since: since, Timestamps: timestamps,
		},
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	defer logs.Close()

	hdr := make([]byte, 8)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if _, err := io.ReadFull(logs, hdr); err != nil {
			if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
				_, err := fmt.Fprintf(resp, "event: done\ndata: {}\n\n")
				if err != nil {
					return err
				}
				flusher.Flush()
			}
			return nil
		}
		size := binary.BigEndian.Uint32(hdr[4:])
		if size == 0 {
			continue
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(logs, payload); err != nil {
			return nil
		}
		streamType := "stdout"
		if hdr[0] == 2 {
			streamType = "stderr"
		}
		data, _ := json.Marshal(map[string]string{"stream": streamType, "line": string(payload)})
		_, err := fmt.Fprintf(resp, "event: log\ndata: %s\n\n", data)
		if err != nil {
			return err
		}
		flusher.Flush()
	}
}

func (h *Handler) Stats(c *echo.Context) error {
	id := c.Param("id")
	resp := c.Response()
	resp.Header().Set("Content-Type", "text/event-stream")
	resp.Header().Set("Cache-Control", "no-cache")
	resp.Header().Set("Connection", "keep-alive")
	resp.Header().Set("X-Accel-Buffering", "no")
	resp.WriteHeader(http.StatusOK)
	flusher, ok := resp.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}

	ctx := c.Request().Context()
	statsStream, err := h.runtime.ContainerStats(ctx, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	defer statsStream.Close()

	decoder := json.NewDecoder(statsStream)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil
		}
		parsed, err := dockerruntime.ParseStats(raw)
		if err != nil {
			continue
		}
		data, _ := json.Marshal(parsed)
		_, err = fmt.Fprintf(resp, "event: stats\ndata: %s\n\n", data)
		if err != nil {
			return err
		}
		flusher.Flush()
	}
}
