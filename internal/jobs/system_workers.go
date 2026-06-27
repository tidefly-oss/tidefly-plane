package jobs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/notification"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/config"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/metrics"
	"gorm.io/gorm"
)

// NOTE: UpdateCheckArgs is defined in update_check_worker.go
// NOTE: fetchRemoteDigest/parseImageRef moved to internal/reconciler/digest.go

type MetricsArgs struct{}

func (MetricsArgs) Kind() string { return "system:metrics" }

type RetentionArgs struct {
	AuditRetentionDays        int `json:"audit_retention_days"`
	NotificationRetentionDays int `json:"notification_retention_days"`
}

func (RetentionArgs) Kind() string { return "system:retention" }

type RuntimeCleanupArgs struct {
	OlderThanHours    float64 `json:"older_than_hours"`
	StoppedContainers bool    `json:"stopped_containers"`
	DanglingImages    bool    `json:"dangling_images"`
	UnusedVolumes     bool    `json:"unused_volumes"`
}

func (RuntimeCleanupArgs) Kind() string { return "system:runtime_cleanup" }

type RuntimeHealthArgs struct{}

func (RuntimeHealthArgs) Kind() string { return "system:runtime_health" }

type SystemWorkers struct {
	rt       runtime.Runtime
	db       *gorm.DB
	log      *logger.Logger
	cfg      config.JobsConfig
	notifSvc *notification.Service
	notifier *notification.Notifier
	metrics  *metrics.Registry
	bus      *eventbus.Bus
}

func newSystemWorkers(
	db *gorm.DB, rt runtime.Runtime, log *logger.Logger, cfg config.JobsConfig,
	notifSvc *notification.Service, notifier *notification.Notifier, metricsReg *metrics.Registry,
	bus *eventbus.Bus,
) *SystemWorkers {
	return &SystemWorkers{rt: rt, db: db, log: log, cfg: cfg, notifSvc: notifSvc, notifier: notifier, metrics: metricsReg, bus: bus}
}

const (
	thresholdCPU  = 90.0
	thresholdMem  = 85.0
	thresholdDisk = 90.0
)

type MetricsWorker struct {
	river.WorkerDefaults[MetricsArgs]
	h *SystemWorkers
}

func (w *MetricsWorker) Work(ctx context.Context, _ *river.Job[MetricsArgs]) error {
	info, err := w.h.rt.SystemInfo(ctx)
	if err != nil {
		return fmt.Errorf("metrics: system info: %w", err)
	}
	w.h.metrics.SetSystem(info.CPUPercent, info.MemUsedMB, info.MemTotalMB, info.MemPercent, info.DiskUsedMB, info.DiskTotalMB, info.DiskPercent)
	w.h.metrics.UpdateRuntime()

	w.h.bus.Publish(eventbus.Event{
		Type:  eventbus.EventSystemMetrics,
		Topic: eventbus.TopicMetrics,
		Payload: eventbus.SystemMetricsPayload{
			CPUPercent: info.CPUPercent,
			MemPercent: info.MemPercent,
			DiskUsed:   info.DiskUsedMB,
			DiskTotal:  info.DiskTotalMB,
		},
	})

	w.h.metrics.IncJob(MetricsArgs{}.Kind())
	if w.h.notifSvc == nil {
		return nil
	}
	if info.CPUPercent >= thresholdCPU {
		_ = w.h.notifSvc.Upsert(ctx, "system", "system", models.SeverityWarn, fmt.Sprintf("High CPU usage: %.1f%%", info.CPUPercent))
	}
	if info.MemPercent >= thresholdMem {
		_ = w.h.notifSvc.Upsert(ctx, "system", "system", models.SeverityWarn, fmt.Sprintf("High memory usage: %.1f%% (%.0f / %.0f MB)", info.MemPercent, float64(info.MemUsedMB), float64(info.MemTotalMB)))
	}
	if info.DiskPercent >= thresholdDisk {
		_ = w.h.notifSvc.Upsert(ctx, "system", "system", models.SeverityWarn, fmt.Sprintf("High disk usage: %.1f%% (%.0f / %.0f GB)", info.DiskPercent, float64(info.DiskUsedMB)/1024, float64(info.DiskTotalMB)/1024))
	}
	return nil
}

type RetentionWorker struct {
	river.WorkerDefaults[RetentionArgs]
	h *SystemWorkers
}

func (w *RetentionWorker) Work(ctx context.Context, job *river.Job[RetentionArgs]) error {
	p := job.Args
	if p.AuditRetentionDays <= 0 {
		p.AuditRetentionDays = 90
	}
	if p.NotificationRetentionDays <= 0 {
		p.NotificationRetentionDays = 30
	}
	infoR := w.h.db.WithContext(ctx).Where("level = 'INFO' AND created_at < ?", time.Now().AddDate(0, 0, -3)).Delete(&models.AppLog{})
	warnR := w.h.db.WithContext(ctx).Where("level = 'WARN' AND created_at < ?", time.Now().AddDate(0, 0, -7)).Delete(&models.AppLog{})
	errR := w.h.db.WithContext(ctx).Where("level = 'ERROR' AND created_at < ?", time.Now().AddDate(0, 0, -30)).Delete(&models.AppLog{})
	auditR := w.h.db.WithContext(ctx).Where("created_at < ?", time.Now().AddDate(0, 0, -p.AuditRetentionDays)).Delete(&models.AuditLog{})
	notifR := w.h.db.WithContext(ctx).Where("acknowledged_at IS NOT NULL AND acknowledged_at < ?", time.Now().AddDate(0, 0, -p.NotificationRetentionDays)).Delete(&models.Notification{})
	for _, r := range []*gorm.DB{infoR, warnR, errR, auditR, notifR} {
		if r.Error != nil {
			return fmt.Errorf("retention: %w", r.Error)
		}
	}
	w.h.log.Info("jobs", fmt.Sprintf("retention: %d INFO / %d WARN / %d ERROR / %d audit / %d notifications deleted",
		infoR.RowsAffected, warnR.RowsAffected, errR.RowsAffected, auditR.RowsAffected, notifR.RowsAffected))
	w.h.metrics.IncJob(RetentionArgs{}.Kind())
	return nil
}

type RuntimeCleanupWorker struct {
	river.WorkerDefaults[RuntimeCleanupArgs]
	h *SystemWorkers
}

func (w *RuntimeCleanupWorker) Work(ctx context.Context, job *river.Job[RuntimeCleanupArgs]) error {
	p := job.Args
	allContainers, err := w.h.rt.ListContainers(ctx, true)
	if err != nil {
		return fmt.Errorf("cleanup: list containers: %w", err)
	}
	usedImages := make(map[string]string)
	usedVolumes := make(map[string]string)
	for _, c := range allContainers {
		usedImages[c.Image] = c.Name
		if details, err := w.h.rt.GetContainer(ctx, c.ID); err == nil {
			for _, m := range details.Mounts {
				if m.Source != "" {
					usedVolumes[m.Source] = c.Name
				}
			}
		}
	}
	cutoff := time.Now().Add(-time.Duration(p.OlderThanHours * float64(time.Hour)))
	var removed, errors int
	if p.StoppedContainers {
		for _, c := range allContainers {
			if c.Status == runtime.StatusRunning || c.Status == runtime.StatusPaused || c.Created.After(cutoff) {
				continue
			}
			if err := w.h.rt.DeleteContainer(ctx, c.ID, false); err != nil {
				errors++
				continue
			}
			removed++
		}
	}
	if p.DanglingImages {
		images, err := w.h.rt.ListImages(ctx)
		if err != nil {
			return fmt.Errorf("cleanup: list images: %w", err)
		}
		for _, img := range images {
			if len(img.Tags) > 0 || img.Created.After(cutoff) {
				continue
			}
			if _, inUse := usedImages[img.ID]; inUse {
				continue
			}
			if err := w.h.rt.DeleteImage(ctx, img.ID, false); err != nil {
				errors++
				continue
			}
			removed++
		}
	}
	if p.UnusedVolumes {
		volumes, err := w.h.rt.ListVolumes(ctx)
		if err != nil {
			return fmt.Errorf("cleanup: list volumes: %w", err)
		}
		for _, v := range volumes {
			if v.CreatedAt.After(cutoff) {
				continue
			}
			if _, inUse := usedVolumes[v.Mountpath]; inUse {
				continue
			}
			if err := w.h.rt.DeleteVolume(ctx, v.Name); err != nil {
				if strings.Contains(err.Error(), "volume is in use") || strings.Contains(err.Error(), "status 409") {
					continue
				}
				errors++
				continue
			}
			removed++
		}
	}
	w.h.log.Info("jobs", fmt.Sprintf("runtime cleanup: removed %d resources, %d errors", removed, errors))
	return nil
}

type RuntimeHealthWorker struct {
	river.WorkerDefaults[RuntimeHealthArgs]
	h *SystemWorkers
}

func (w *RuntimeHealthWorker) Work(ctx context.Context, _ *river.Job[RuntimeHealthArgs]) error {
	containers, err := w.h.rt.ListContainers(ctx, true)
	if err != nil {
		return fmt.Errorf("healthcheck: list containers: %w", err)
	}
	for _, c := range containers {
		if c.Status == runtime.StatusDead {
			w.h.log.ContainerEvent("WARN", c.ID, c.Name, "container exited unexpectedly", "")
		}
	}
	return nil
}

// ── Webhook update helpers ────────────────────────────────────────────────────

func MarkUpdateAvailableByTemplateSlug(ctx context.Context, db *gorm.DB, slug string) (int64, error) {
	r := db.WithContext(ctx).Model(&models.Service{}).
		Where("template_slug = ? AND status = ?", slug, models.ServiceStatusRunning).
		Updates(map[string]any{"update_available": true, "update_source": models.UpdateSourceTemplate})
	return r.RowsAffected, r.Error
}

func MarkUpdateAvailableByServiceID(ctx context.Context, db *gorm.DB, serviceID string) error {
	return db.WithContext(ctx).Model(&models.Service{}).Where("id = ?", serviceID).
		Updates(map[string]any{"update_available": true, "update_source": models.UpdateSourceGit}).Error
}

func ResetUpdateAvailable(ctx context.Context, db *gorm.DB, serviceID string) error {
	return db.WithContext(ctx).Model(&models.Service{}).Where("id = ?", serviceID).
		Updates(map[string]any{"update_available": false, "update_source": ""}).Error
}
