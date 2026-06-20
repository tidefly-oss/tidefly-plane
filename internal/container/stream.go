package container

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
)

func (h *Handler) Logs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "100"
	}
	since := r.URL.Query().Get("since")
	timestamps, _ := strconv.ParseBool(r.URL.Query().Get("timestamps"))

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	logs, err := h.runtime.ContainerLogs(ctx, id, runtime.LogOptions{
		Follow: true, Tail: tail, Since: since, Timestamps: timestamps,
	})
	if err != nil {
		_, _ = fmt.Fprintf(w, "event: error\ndata: {\"error\":\"%s\"}\n\n", err.Error())
		flusher.Flush()
		return
	}
	defer func() {
		if err := logs.Close(); err != nil {
			h.log.Error("streams", "failed to close logs", err)
		}
	}()

	hdr := make([]byte, 8)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if _, err := io.ReadFull(logs, hdr); err != nil {
			if err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
				_, _ = fmt.Fprintf(w, "event: done\ndata: {}\n\n")
				flusher.Flush()
			}
			return
		}
		size := binary.BigEndian.Uint32(hdr[4:])
		if size == 0 {
			continue
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(logs, payload); err != nil {
			return
		}
		streamType := "stdout"
		if hdr[0] == 2 {
			streamType = "stderr"
		}
		data, _ := json.Marshal(map[string]string{"stream": streamType, "line": string(payload)})
		if _, err := fmt.Fprintf(w, "event: log\ndata: %s\n\n", data); err != nil {
			return
		}
		flusher.Flush()
	}
}

func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rc, err := h.runtime.ContainerStats(ctx, id)
			if err != nil {
				_, _ = fmt.Fprintf(w, "event: error\ndata: {\"error\":\"%s\"}\n\n", err.Error())
				flusher.Flush()
				return
			}
			buf := make([]byte, 65536)
			n, _ := rc.Read(buf)
			_ = rc.Close()
			if n == 0 {
				continue
			}
			entry := parseStats(buf[:n])
			if entry == nil {
				continue
			}
			data, _ := json.Marshal(entry)
			if _, err := fmt.Fprintf(w, "event: stats\ndata: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// ── Stats parsing ─────────────────────────────────────────────────────────────

type statsEntry struct {
	CPUPercent   float64 `json:"cpu_percent"`
	MemUsageMB   float64 `json:"mem_usage_mb"`
	MemLimitMB   float64 `json:"mem_limit_mb"`
	MemPercent   float64 `json:"mem_percent"`
	NetworkRxMB  float64 `json:"network_rx_mb"`
	NetworkTxMB  float64 `json:"network_tx_mb"`
	BlockReadMB  float64 `json:"block_read_mb"`
	BlockWriteMB float64 `json:"block_write_mb"`
	PIDs         int     `json:"pids"`
}

type dockerStatsRaw struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     int    `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64            `json:"usage"`
		Limit uint64            `json:"limit"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
	BlkioStats struct {
		IOServiceBytesRecursive []struct {
			Op    string `json:"op"`
			Value uint64 `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
	PidsStats struct {
		Current int `json:"current"`
	} `json:"pids_stats"`
}

func parseStats(raw []byte) *statsEntry {
	var ds dockerStatsRaw
	if err := json.Unmarshal(raw, &ds); err != nil {
		return nil
	}

	cpuDelta := float64(ds.CPUStats.CPUUsage.TotalUsage) - float64(ds.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(ds.CPUStats.SystemCPUUsage) - float64(ds.PreCPUStats.SystemCPUUsage)
	cpus := ds.CPUStats.OnlineCPUs
	if cpus == 0 {
		cpus = 1
	}
	var cpuPct float64
	if sysDelta > 0 {
		cpuPct = (cpuDelta / sysDelta) * float64(cpus) * 100
	}

	cache := ds.MemoryStats.Stats["cache"]
	if cache == 0 {
		cache = ds.MemoryStats.Stats["inactive_file"]
	}
	memUsage := float64(ds.MemoryStats.Usage-cache) / 1024 / 1024
	memLimit := float64(ds.MemoryStats.Limit) / 1024 / 1024
	var memPct float64
	if ds.MemoryStats.Limit > 0 {
		memPct = float64(ds.MemoryStats.Usage-cache) / float64(ds.MemoryStats.Limit) * 100
	}

	var rxBytes, txBytes uint64
	for _, n := range ds.Networks {
		rxBytes += n.RxBytes
		txBytes += n.TxBytes
	}

	var blockRead, blockWrite uint64
	for _, b := range ds.BlkioStats.IOServiceBytesRecursive {
		switch b.Op {
		case "Read":
			blockRead += b.Value
		case "Write":
			blockWrite += b.Value
		}
	}

	return &statsEntry{
		CPUPercent:   cpuPct,
		MemUsageMB:   memUsage,
		MemLimitMB:   memLimit,
		MemPercent:   memPct,
		NetworkRxMB:  float64(rxBytes) / 1024 / 1024,
		NetworkTxMB:  float64(txBytes) / 1024 / 1024,
		BlockReadMB:  float64(blockRead) / 1024 / 1024,
		BlockWriteMB: float64(blockWrite) / 1024 / 1024,
		PIDs:         ds.PidsStats.Current,
	}
}
