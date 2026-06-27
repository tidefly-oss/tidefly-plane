package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
	"github.com/tidefly-oss/tidefly-plane/internal/agent"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/ingress"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/notification"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/config"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/metrics"
	"github.com/tidefly-oss/tidefly-plane/internal/reconciler"
	"gorm.io/gorm"
)

const (
	queueCritical = "critical"
	queueLow      = "low"
	queueDefault  = river.QueueDefault
)

type Server struct {
	river  *river.Client[pgx.Tx]
	pool   *pgxpool.Pool
	cfg    config.JobsConfig
	log    *logger.Logger
	svc    *ServiceWorkers
	system *SystemWorkers
	stops  *stopTracker
}

func NewServer(
	pool *pgxpool.Pool,
	cfg config.JobsConfig,
	rt runtime.Runtime,
	db *gorm.DB,
	log *logger.Logger,
	notifSvc *notification.Service,
	notifier *notification.Notifier,
	metricsReg *metrics.Registry,
	ingressAdapter ingress.Adapter,
	agentClient *agent.Client,
	rec *reconciler.Reconciler,
	bus *eventbus.Bus,
) (*Server, error) {
	workers := river.NewWorkers()

	svc := newServiceWorkers(db, rt, ingressAdapter, log, notifSvc, notifier, agentClient, bus)
	sys := newSystemWorkers(db, rt, log, cfg, notifSvc, notifier, metricsReg, bus)

	river.AddWorker(workers, &DeployWorker{h: svc})
	river.AddWorker(workers, &RedeployWorker{h: svc})
	river.AddWorker(workers, &UpdateWorker{h: svc})
	river.AddWorker(workers, &DeleteWorker{h: svc})
	river.AddWorker(workers, &HealWorker{h: svc})
	river.AddWorker(workers, &CleanupWorker{h: svc})
	river.AddWorker(workers, &HealthCheckWorker{h: svc})
	river.AddWorker(workers, &AutoscaleWorker{h: svc})
	river.AddWorker(workers, &MetricsWorker{h: sys})
	river.AddWorker(workers, &RetentionWorker{h: sys})
	river.AddWorker(workers, &RuntimeCleanupWorker{h: sys})
	river.AddWorker(workers, &RuntimeHealthWorker{h: sys})
	river.AddWorker(workers, NewUpdateCheckWorker(rec))

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			queueDefault:  {MaxWorkers: cfg.Concurrency},
			queueCritical: {MaxWorkers: cfg.Concurrency},
			queueLow:      {MaxWorkers: 2},
		},
		Workers:      workers,
		PeriodicJobs: buildPeriodicJobs(cfg),
		ErrorHandler: &riverErrorHandler{log: log, metrics: metricsReg},
	})
	if err != nil {
		return nil, fmt.Errorf("jobs: init river: %w", err)
	}

	return &Server{
		river:  riverClient,
		pool:   pool,
		cfg:    cfg,
		log:    log,
		svc:    svc,
		system: sys,
		stops:  newStopTracker(),
	}, nil
}

func (s *Server) Run(ctx context.Context) error {
	if err := s.river.Start(ctx); err != nil {
		return fmt.Errorf("jobs: river start: %w", err)
	}
	go s.watchContainerEvents(ctx)
	s.log.Info("jobs", fmt.Sprintf("river job server started (runtime: %s)", s.system.rt.Type()))

	<-ctx.Done()
	s.log.Info("jobs", "jobs server stopping")

	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.river.Stop(stopCtx)
}

func (s *Server) Client() *river.Client[pgx.Tx] {
	return s.river
}

func buildPeriodicJobs(cfg config.JobsConfig) []*river.PeriodicJob {
	return []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(15*time.Second),
			func() (river.JobArgs, *river.InsertOpts) {
				return MetricsArgs{}, &river.InsertOpts{Queue: queueCritical}
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(30*time.Second),
			func() (river.JobArgs, *river.InsertOpts) {
				return HealthCheckArgs{}, &river.InsertOpts{Queue: queueCritical, UniqueOpts: river.UniqueOpts{ByArgs: true}}
			},
			nil,
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(30*time.Second),
			func() (river.JobArgs, *river.InsertOpts) {
				return AutoscaleArgs{}, &river.InsertOpts{Queue: queueCritical, UniqueOpts: river.UniqueOpts{ByArgs: true}}
			},
			nil,
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(6*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return UpdateCheckArgs{}, &river.InsertOpts{Queue: queueLow, UniqueOpts: river.UniqueOpts{ByArgs: true}}
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),
		river.NewPeriodicJob(
			intervalFromCron(cfg.CleanupCron, 24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return RuntimeCleanupArgs{
					OlderThanHours:    cfg.CleanupOlderThan.Hours(),
					StoppedContainers: cfg.CleanupStoppedContainers,
					DanglingImages:    cfg.CleanupDanglingImages,
					UnusedVolumes:     cfg.CleanupUnusedVolumes,
				}, &river.InsertOpts{Queue: queueLow}
			},
			nil,
		),
		river.NewPeriodicJob(
			intervalFromCron(cfg.LogRetentionCron, 24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return RetentionArgs{
					AuditRetentionDays:        cfg.AuditRetentionDays,
					NotificationRetentionDays: cfg.NotificationRetentionDays,
				}, &river.InsertOpts{Queue: queueLow}
			},
			nil,
		),
		river.NewPeriodicJob(
			intervalFromCron(cfg.HealthCheckCron, 2*time.Minute),
			func() (river.JobArgs, *river.InsertOpts) {
				return RuntimeHealthArgs{}, &river.InsertOpts{Queue: queueCritical}
			},
			nil,
		),
	}
}

func intervalFromCron(expr string, fallback time.Duration) river.PeriodicSchedule {
	switch expr {
	case "@hourly", "0 * * * *":
		return river.PeriodicInterval(time.Hour)
	case "@daily", "0 0 * * *":
		return river.PeriodicInterval(24 * time.Hour)
	case "@weekly", "0 0 * * 0":
		return river.PeriodicInterval(7 * 24 * time.Hour)
	case "@every 6h":
		return river.PeriodicInterval(6 * time.Hour)
	case "@every 12h":
		return river.PeriodicInterval(12 * time.Hour)
	case "@every 30m":
		return river.PeriodicInterval(30 * time.Minute)
	default:
		return river.PeriodicInterval(fallback)
	}
}

type riverErrorHandler struct {
	log     *logger.Logger
	metrics *metrics.Registry
}

func (h *riverErrorHandler) HandleError(ctx context.Context, job *rivertype.JobRow, err error) *river.ErrorHandlerResult {
	h.log.Error("jobs", fmt.Sprintf("job %s (id=%d) failed (attempt %d)", job.Kind, job.ID, job.Attempt), err)
	if h.metrics != nil {
		h.metrics.IncJobFailed(job.Kind, err)
	}
	return nil
}

func (h *riverErrorHandler) HandlePanic(ctx context.Context, job *rivertype.JobRow, panicVal any, trace string) *river.ErrorHandlerResult {
	h.log.Error("jobs", fmt.Sprintf("job %s (id=%d) panicked: %v\n%s", job.Kind, job.ID, panicVal, trace), nil)
	return nil
}
