package jobs

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

const TaskMetricsCollect = "metrics:collect"

const (
	thresholdCPU  = 90.0
	thresholdMem  = 85.0
	thresholdDisk = 90.0
)

func (h *Handler) HandleMetricsCollect(ctx context.Context, _ *asynq.Task) error {
	info, err := h.rt.SystemInfo(ctx)
	if err != nil {
		return fmt.Errorf("metrics: system info: %w", err)
	}

	// Update Prometheus gauges — no DB write
	h.metrics.SetSystem(
		info.CPUPercent,
		info.MemUsedMB, info.MemTotalMB, info.MemPercent,
		info.DiskUsedMB, info.DiskTotalMB, info.DiskPercent,
	)
	h.metrics.UpdateRuntime()
	h.metrics.IncJob(TaskMetricsCollect)

	if h.notifSvc != nil {
		if info.CPUPercent >= thresholdCPU {
			_ = h.notifSvc.Upsert(
				ctx, "system", "system",
				models.SeverityWarn,
				fmt.Sprintf("High CPU usage: %.1f%%", info.CPUPercent),
			)
		}
		if info.MemPercent >= thresholdMem {
			_ = h.notifSvc.Upsert(
				ctx, "system", "system",
				models.SeverityWarn,
				fmt.Sprintf(
					"High memory usage: %.1f%% (%.0f / %.0f MB)",
					info.MemPercent, float64(info.MemUsedMB), float64(info.MemTotalMB),
				),
			)
		}
		if info.DiskPercent >= thresholdDisk {
			_ = h.notifSvc.Upsert(
				ctx, "system", "system",
				models.SeverityWarn,
				fmt.Sprintf(
					"High disk usage: %.1f%% (%.0f / %.0f GB)",
					info.DiskPercent,
					float64(info.DiskUsedMB)/1024,
					float64(info.DiskTotalMB)/1024,
				),
			)
		}
	}

	return nil
}
