package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/riverqueue/river"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/notification"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_logger"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/config"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/metrics"
	"gorm.io/gorm"
)

// ── Job Arg Types ─────────────────────────────────────────────────────────────

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

type UpdateCheckArgs struct{}

func (UpdateCheckArgs) Kind() string { return "system:update_check" }

// ── Shared handler ────────────────────────────────────────────────────────────

type SystemWorkers struct {
	rt       runtime.Runtime
	db       *gorm.DB
	log      *_logger.Logger
	cfg      config.JobsConfig
	notifSvc *notification.Service
	notifier *notification.Notifier
	metrics  *metrics.Registry
}

func newSystemWorkers(
	db *gorm.DB,
	rt runtime.Runtime,
	log *_logger.Logger,
	cfg config.JobsConfig,
	notifSvc *notification.Service,
	notifier *notification.Notifier,
	metricsReg *metrics.Registry,
) *SystemWorkers {
	return &SystemWorkers{rt: rt, db: db, log: log, cfg: cfg, notifSvc: notifSvc, notifier: notifier, metrics: metricsReg}
}

// ── Metrics ───────────────────────────────────────────────────────────────────

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
	w.h.metrics.SetSystem(
		info.CPUPercent,
		info.MemUsedMB, info.MemTotalMB, info.MemPercent,
		info.DiskUsedMB, info.DiskTotalMB, info.DiskPercent,
	)
	w.h.metrics.UpdateRuntime()
	w.h.metrics.IncJob(MetricsArgs{}.Kind())

	if w.h.notifSvc == nil {
		return nil
	}
	if info.CPUPercent >= thresholdCPU {
		_ = w.h.notifSvc.Upsert(ctx, "system", "system", models.SeverityWarn,
			fmt.Sprintf("High CPU usage: %.1f%%", info.CPUPercent))
	}
	if info.MemPercent >= thresholdMem {
		_ = w.h.notifSvc.Upsert(ctx, "system", "system", models.SeverityWarn,
			fmt.Sprintf("High memory usage: %.1f%% (%.0f / %.0f MB)", info.MemPercent, float64(info.MemUsedMB), float64(info.MemTotalMB)))
	}
	if info.DiskPercent >= thresholdDisk {
		_ = w.h.notifSvc.Upsert(ctx, "system", "system", models.SeverityWarn,
			fmt.Sprintf("High disk usage: %.1f%% (%.0f / %.0f GB)", info.DiskPercent, float64(info.DiskUsedMB)/1024, float64(info.DiskTotalMB)/1024))
	}
	return nil
}

// ── Log / Notification Retention ─────────────────────────────────────────────

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
	notifR := w.h.db.WithContext(ctx).
		Where("acknowledged_at IS NOT NULL AND acknowledged_at < ?", time.Now().AddDate(0, 0, -p.NotificationRetentionDays)).
		Delete(&models.Notification{})

	for _, r := range []*gorm.DB{infoR, warnR, errR, auditR, notifR} {
		if r.Error != nil {
			return fmt.Errorf("retention: %w", r.Error)
		}
	}
	w.h.log.Info("jobs", fmt.Sprintf("retention: deleted %d INFO / %d WARN / %d ERROR app logs, %d audit, %d notifications",
		infoR.RowsAffected, warnR.RowsAffected, errR.RowsAffected, auditR.RowsAffected, notifR.RowsAffected))
	w.h.metrics.IncJob(RetentionArgs{}.Kind())
	return nil
}

// ── Runtime Cleanup ───────────────────────────────────────────────────────────

type RuntimeCleanupWorker struct {
	river.WorkerDefaults[RuntimeCleanupArgs]
	h *SystemWorkers
}

func (w *RuntimeCleanupWorker) Work(ctx context.Context, job *river.Job[RuntimeCleanupArgs]) error {
	p := job.Args
	w.h.log.Info("jobs", fmt.Sprintf("runtime cleanup started (runtime: %s)", w.h.rt.Type()))

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

	w.h.log.Info("jobs", fmt.Sprintf("runtime cleanup finished: removed %d resources, %d errors", removed, errors))
	return nil
}

// ── Runtime Health ────────────────────────────────────────────────────────────

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
		if c.Status == runtime.StatusExited {
			w.h.log.ContainerEvent("WARN", c.ID, c.Name, "container exited unexpectedly", "")
		}
	}
	return nil
}

// ── Update Checker ────────────────────────────────────────────────────────────

type updateCheckResult struct {
	serviceID   string
	serviceName string
	digest      string
	hasUpdate   bool
	err         error
}

type UpdateCheckWorker struct {
	river.WorkerDefaults[UpdateCheckArgs]
	h *SystemWorkers
}

func (w *UpdateCheckWorker) Work(ctx context.Context, _ *river.Job[UpdateCheckArgs]) error {
	w.h.log.Info("jobs", "update_checker: starting digest check")

	var services []models.Service
	if err := w.h.db.WithContext(ctx).Where("status = ?", models.ServiceStatusRunning).Find(&services).Error; err != nil {
		return fmt.Errorf("update_checker: list services: %w", err)
	}
	if len(services) == 0 {
		return nil
	}

	sem := make(chan struct{}, 3)
	results := make(chan updateCheckResult, len(services))
	var wg sync.WaitGroup

	for _, svc := range services {
		image := resolveServiceImage(svc)
		if image == "" {
			continue
		}
		wg.Add(1)
		go func(s models.Service, img string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			digest, err := fetchRemoteDigest(ctx, img)
			if err != nil {
				results <- updateCheckResult{serviceID: s.ID.String(), err: err}
				return
			}
			results <- updateCheckResult{
				serviceID:   s.ID.String(),
				serviceName: s.Name,
				digest:      digest,
				hasUpdate:   digest != "" && digest != s.RemoteDigest,
			}
		}(svc, image)
	}
	go func() { wg.Wait(); close(results) }()

	now := time.Now().UTC()
	updated := 0
	for r := range results {
		if r.err != nil {
			w.h.log.Info("jobs", fmt.Sprintf("update_checker: %s digest fetch failed: %v", r.serviceID, r.err))
			continue
		}
		fields := map[string]any{"remote_digest": r.digest, "update_checked_at": now}
		if r.hasUpdate {
			fields["update_available"] = true
			fields["update_source"] = models.UpdateSourceRegistry
		}
		if err := w.h.db.WithContext(ctx).Model(&models.Service{}).Where("id = ?", r.serviceID).Updates(fields).Error; err != nil {
			continue
		}
		if r.hasUpdate {
			updated++
			if w.h.notifSvc != nil {
				_ = w.h.notifSvc.Publish(ctx, models.SeverityInfo, "Update verfügbar",
					fmt.Sprintf("Service \"%s\" hat ein neues Image verfügbar.", r.serviceName))
			}
		}
	}
	w.h.log.Info("jobs", fmt.Sprintf("update_checker: %d/%d services have updates", updated, len(services)))
	return nil
}

// Helpers shared with HTTP handlers (exported so update webhook handler can call them directly).

func ResolveServiceImage(svc models.Service) string { return resolveServiceImage(svc) }

func resolveServiceImage(svc models.Service) string {
	if svc.ManifestJSON == "" {
		return ""
	}
	var m struct {
		Image string `json:"image"`
	}
	if err := json.Unmarshal([]byte(svc.ManifestJSON), &m); err != nil {
		return ""
	}
	return m.Image
}

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

func fetchRemoteDigest(ctx context.Context, image string) (string, error) {
	registry, repo, tag := parseImageRef(image)
	token, err := fetchDockerHubToken(ctx, registry, repo)
	if err != nil {
		return "", fmt.Errorf("auth token: %w", err)
	}
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry returned %d for %s", resp.StatusCode, image)
	}
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("no Docker-Content-Digest header for %s", image)
	}
	return digest, nil
}

func fetchDockerHubToken(ctx context.Context, registry, repo string) (string, error) {
	if registry != "registry-1.docker.io" {
		return "", nil
	}
	url := fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull", repo)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(body, &result)
	return result.Token, nil
}

func parseImageRef(image string) (registry, repo, tag string) {
	tag = "latest"
	if i := strings.LastIndex(image, ":"); i != -1 && !strings.Contains(image[i:], "/") {
		tag = image[i+1:]
		image = image[:i]
	}
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 2 && strings.ContainsAny(parts[0], ".:") {
		registry = parts[0]
		repo = parts[1]
	} else {
		registry = "registry-1.docker.io"
		if len(parts) == 1 {
			repo = "library/" + parts[0]
		} else {
			repo = image
		}
	}
	return
}
