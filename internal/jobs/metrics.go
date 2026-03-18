package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/tidefly-oss/tidefly-backend/internal/models"
	"github.com/hibiken/asynq"
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

	metric := models.SystemMetric{
		CPUPercent:  info.CPUPercent,
		MemUsedMB:   info.MemUsedMB,
		MemTotalMB:  info.MemTotalMB,
		MemPercent:  info.MemPercent,
		DiskUsedMB:  info.DiskUsedMB,
		DiskTotalMB: info.DiskTotalMB,
		DiskPercent: info.DiskPercent,
		CollectedAt: time.Now(),
	}

	if err := h.db.WithContext(ctx).Create(&metric).Error; err != nil {
		return fmt.Errorf("metrics: save: %w", err)
	}

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
