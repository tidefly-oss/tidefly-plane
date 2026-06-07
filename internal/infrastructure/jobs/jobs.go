// Package jobs implements background job handlers, schedulers, and the container event watcher.
package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/notification"
	agentsvc "github.com/tidefly-oss/tidefly-plane/internal/infrastructure/agent"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/ingress"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/metrics"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/config"
)

const (
	TaskRuntimeCleanup     = "runtime:cleanup"
	TaskRuntimeHealthCheck = "runtime:healthcheck"
	TaskLogsRetention      = "logs:retention"
	TaskServiceHealthCheck = "services:healthcheck"
	TaskServiceAutoscale   = "services:autoscale"
)

type Server struct {
	server     *asynq.Server
	scheduler  *asynq.Scheduler
	client     *asynq.Client
	handler    *Handler
	svcHandler *ServiceJobHandler
	cfg        config.JobsConfig
	log        *logger.Logger
}

type Handler struct {
	rt       runtime.Runtime
	db       *gorm.DB
	log      *logger.Logger
	cfg      config.JobsConfig
	notifSvc *notification.Service
	metrics  *metrics.Registry
}

func NewServer(
	redisCfg config.RedisConfig,
	jobsCfg config.JobsConfig,
	rt runtime.Runtime,
	db *gorm.DB,
	log *logger.Logger,
	notifySvc *notification.Service,
	metricsReg *metrics.Registry,
	ingressAdapter ingress.Adapter,
	agentClient *agentsvc.Client,
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

	svcHandler := NewServiceJobHandler(db, rt, ingressAdapter, log, client, agentClient)

	return &Server{
		server:     srv,
		scheduler:  scheduler,
		client:     client,
		handler:    h,
		svcHandler: svcHandler,
		cfg:        jobsCfg,
		log:        log,
	}, nil
}

func (s *Server) Run(ctx context.Context) error {
	mux := asynq.NewServeMux()

	mux.HandleFunc(TaskServiceCleanup, s.svcHandler.HandleServiceCleanup)
	mux.HandleFunc(TaskRuntimeHealthCheck, s.handler.HandleRuntimeHealthCheck)
	mux.HandleFunc(TaskRuntimeCleanup, s.handler.HandleRuntimeCleanup)
	mux.HandleFunc(TaskLogsRetention, s.handler.HandleLogsRetention)
	mux.HandleFunc(TaskMetricsCollect, s.handler.HandleMetricsCollect)

	mux.HandleFunc(TaskServiceDeploy, s.svcHandler.HandleServiceDeploy)
	mux.HandleFunc(TaskServiceRedeploy, s.svcHandler.HandleServiceRedeploy)
	mux.HandleFunc(TaskServiceUpdate, s.svcHandler.HandleServiceUpdate)
	mux.HandleFunc(TaskServiceDelete, s.svcHandler.HandleServiceDelete)

	mux.HandleFunc(TaskServiceHealthCheck, s.svcHandler.HandleServiceHealthCheck)
	mux.HandleFunc(TaskServiceAutoscale, s.svcHandler.HandleServiceAutoscale)
	mux.HandleFunc(TaskServiceHeal, s.svcHandler.HandleServiceHeal)

	if err := s.registerSchedules(); err != nil {
		return fmt.Errorf("jobs: register schedules: %w", err)
	}
	if err := s.scheduler.Start(); err != nil {
		return fmt.Errorf("jobs: start scheduler: %w", err)
	}

	go s.WatchContainerEvents(ctx)

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

	serviceHealthTask, err := newTask(TaskServiceHealthCheck, nil)
	if err != nil {
		return err
	}
	if _, err := s.scheduler.Register(
		"@every 30s", serviceHealthTask,
		asynq.Queue("critical"), asynq.MaxRetry(0), asynq.Timeout(30*time.Second),
	); err != nil {
		return fmt.Errorf("register service healthcheck: %w", err)
	}

	autoscaleTask, err := newTask(TaskServiceAutoscale, nil)
	if err != nil {
		return err
	}
	if _, err := s.scheduler.Register(
		"@every 30s", autoscaleTask,
		asynq.Queue("critical"), asynq.MaxRetry(0), asynq.Timeout(25*time.Second),
	); err != nil {
		return fmt.Errorf("register autoscale: %w", err)
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
