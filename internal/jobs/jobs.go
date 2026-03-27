package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/config"
	"github.com/tidefly-oss/tidefly-plane/internal/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/metrics"
	"github.com/tidefly-oss/tidefly-plane/internal/services/notifications"
	"github.com/tidefly-oss/tidefly-plane/internal/services/runtime"
)

const (
	TaskRuntimeCleanup     = "runtime:cleanup"
	TaskRuntimeHealthCheck = "runtime:healthcheck"
	TaskLogsRetention      = "logs:retention"
)

type Server struct {
	server    *asynq.Server
	scheduler *asynq.Scheduler
	client    *asynq.Client
	handler   *Handler
	cfg       config.JobsConfig
	log       *logger.Logger
}

type Handler struct {
	rt       runtime.Runtime
	db       *gorm.DB
	log      *logger.Logger
	cfg      config.JobsConfig
	notifSvc *notifications.Service
	metrics  *metrics.Registry
}

func NewServer(
	redisCfg config.RedisConfig,
	jobsCfg config.JobsConfig,
	rt runtime.Runtime,
	db *gorm.DB,
	log *logger.Logger,
	notifySvc *notifications.Service,
	metricsReg *metrics.Registry,
) (*Server, error) {
	redisOpt := asynq.RedisClientOpt{
		Addr:     redisCfg.Addr,
		Username: redisCfg.User,
		Password: redisCfg.Password,
	}

	srv := asynq.NewServer(
		redisOpt, asynq.Config{
			Concurrency: jobsCfg.Concurrency,
			Queues: map[string]int{
				"critical": 6,
				"default":  3,
				"low":      1,
			},
			ErrorHandler: asynq.ErrorHandlerFunc(
				func(ctx context.Context, task *asynq.Task, err error) {
					log.Error("jobs", fmt.Sprintf("job %s failed", task.Type()), err)
					metricsReg.IncJobFailed(task.Type(), err)
				},
			),
		},
	)

	scheduler := asynq.NewScheduler(redisOpt, nil)
	client := asynq.NewClient(redisOpt)
	h := &Handler{
		rt:       rt,
		db:       db,
		log:      log,
		cfg:      jobsCfg,
		notifSvc: notifySvc,
		metrics:  metricsReg,
	}

	return &Server{
		server:    srv,
		scheduler: scheduler,
		client:    client,
		handler:   h,
		cfg:       jobsCfg,
		log:       log,
	}, nil
}

func (s *Server) Run(ctx context.Context) error {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskRuntimeCleanup, s.handler.HandleRuntimeCleanup)
	mux.HandleFunc(TaskRuntimeHealthCheck, s.handler.HandleRuntimeHealthCheck)
	mux.HandleFunc(TaskLogsRetention, s.handler.HandleLogsRetention)
	mux.HandleFunc(TaskMetricsCollect, s.handler.HandleMetricsCollect)

	if err := s.registerSchedules(); err != nil {
		return fmt.Errorf("jobs: register schedules: %w", err)
	}
	if err := s.scheduler.Start(); err != nil {
		return fmt.Errorf("jobs: start scheduler: %w", err)
	}

	s.log.Info("jobs", fmt.Sprintf("background jobs started (runtime: %s)", s.handler.rt.Type()))

	errCh := make(chan error, 1)
	go func() { errCh <- s.server.Run(mux) }()

	select {
	case <-ctx.Done():
		s.log.Info("jobs", "background jobs stopping")
		s.scheduler.Shutdown()
		s.server.Shutdown()
		_ = s.client.Close()
		return nil
	case err := <-errCh:
		return fmt.Errorf("jobs: server error: %w", err)
	}
}

func (s *Server) registerSchedules() error {
	// ── Runtime Cleanup ──────────────────────────────────────────────────────
	cleanupTask, err := newTask(
		TaskRuntimeCleanup, map[string]any{
			"older_than_hours":   s.cfg.CleanupOlderThan.Hours(),
			"stopped_containers": s.cfg.CleanupStoppedContainers,
			"dangling_images":    s.cfg.CleanupDanglingImages,
			"unused_volumes":     s.cfg.CleanupUnusedVolumes,
		},
	)
	if err != nil {
		return err
	}
	if _, err := s.scheduler.Register(
		s.cfg.CleanupCron, cleanupTask,
		asynq.Queue("low"), asynq.MaxRetry(2), asynq.Timeout(10*time.Minute),
	); err != nil {
		return fmt.Errorf("register cleanup: %w", err)
	}

	// ── Health Check ─────────────────────────────────────────────────────────
	healthTask, err := newTask(TaskRuntimeHealthCheck, nil)
	if err != nil {
		return err
	}
	if _, err := s.scheduler.Register(
		s.cfg.HealthCheckCron, healthTask,
		asynq.Queue("critical"), asynq.MaxRetry(1), asynq.Timeout(2*time.Minute),
	); err != nil {
		return fmt.Errorf("register healthcheck: %w", err)
	}

	// ── Log / Data Retention ─────────────────────────────────────────────────
	retentionTask, err := newTask(
		TaskLogsRetention, map[string]any{
			"log_retention_days":          s.cfg.LogRetentionDays,
			"audit_retention_days":        s.cfg.AuditRetentionDays,
			"notification_retention_days": s.cfg.NotificationRetentionDays,
		},
	)
	if err != nil {
		return err
	}
	if _, err := s.scheduler.Register(
		s.cfg.LogRetentionCron, retentionTask,
		asynq.Queue("low"), asynq.MaxRetry(1), asynq.Timeout(5*time.Minute),
	); err != nil {
		return fmt.Errorf("register log retention: %w", err)
	}

	// ── Metrics Collect ───────────────────────────────────────────────────────
	metricsTask, err := newTask(TaskMetricsCollect, nil)
	if err != nil {
		return err
	}
	if _, err := s.scheduler.Register(
		"@every 15s", metricsTask,
		asynq.Queue("critical"), asynq.MaxRetry(0), asynq.Timeout(10*time.Second),
	); err != nil {
		return fmt.Errorf("register metrics collect: %w", err)
	}

	return nil
}

func (s *Server) EnqueueNow(taskType string) error {
	task, err := newTask(taskType, nil)
	if err != nil {
		return err
	}
	_, err = s.client.Enqueue(task, asynq.Queue("default"))
	return err
}

func newTask(taskType string, payload map[string]any) (*asynq.Task, error) {
	var data []byte
	var err error
	if payload != nil {
		data, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	return asynq.NewTask(taskType, data), nil
}
