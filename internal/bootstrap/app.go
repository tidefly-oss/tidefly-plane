package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/aarondl/authboss/v3"
	"github.com/danielgtaylor/huma/v2"
	"github.com/hibiken/asynq"
	"github.com/labstack/echo/v5"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/api/adapter"
	"github.com/tidefly-oss/tidefly-backend/internal/api/middleware"
	v1 "github.com/tidefly-oss/tidefly-backend/internal/api/v1"
	"github.com/tidefly-oss/tidefly-backend/internal/config"
	"github.com/tidefly-oss/tidefly-backend/internal/jobs"
	applogger "github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/services/git"
	"github.com/tidefly-oss/tidefly-backend/internal/services/logwatcher"
	"github.com/tidefly-oss/tidefly-backend/internal/services/notifications"
	notifiersvc "github.com/tidefly-oss/tidefly-backend/internal/services/notifier"
	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
	"github.com/tidefly-oss/tidefly-backend/internal/services/template"
	"github.com/tidefly-oss/tidefly-backend/internal/services/webhook"
)

type App struct {
	cfg         *config.Config
	log         *applogger.Logger
	rt          runtime.Runtime
	db          *gorm.DB
	ab          *authboss.Authboss
	templateLd  *template.Loader
	notifSvc    *notifications.Service
	gitSvc      *git.Service
	webhookSvc  *webhook.Service
	jobServer   *jobs.Server
	asynq       *asynq.Client
	notifierSvc *notifiersvc.Service
}

func NewApp(
	cfg *config.Config,
	log *applogger.Logger,
	rt runtime.Runtime,
	db *gorm.DB,
	ab *authboss.Authboss,
	templateLd *template.Loader,
	notifSvc *notifications.Service,
	gitSvc *git.Service,
	webhookSvc *webhook.Service,
	jobServer *jobs.Server,
	asynqClient *asynq.Client,
	notifierSvc *notifiersvc.Service,
) *App {
	return &App{
		cfg: cfg, log: log, rt: rt, db: db,
		ab: ab, templateLd: templateLd, notifSvc: notifSvc,
		gitSvc: gitSvc, webhookSvc: webhookSvc,
		jobServer: jobServer, asynq: asynqClient,
		notifierSvc: notifierSvc,
	}
}

func (a *App) Run(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)

	a.startBackgroundServices(eg, ctx)

	e := a.buildEcho()
	eg.Go(func() error { return a.serveHTTP(ctx, e) })

	return eg.Wait()
}

func (a *App) startBackgroundServices(eg *errgroup.Group, ctx context.Context) {
	if a.cfg.LogWatcher.Enabled {
		watcher := logwatcher.New(a.rt, a.log, a.cfg.LogWatcher, a.notifSvc, a.notifierSvc)
		eg.Go(func() error { watcher.Run(ctx); return nil })
		a.log.Info("app", "container log watcher enabled")
	}
	if a.jobServer != nil {
		eg.Go(func() error { return a.jobServer.Run(ctx) })
		a.log.Info(
			"app", fmt.Sprintf(
				"background jobs enabled (cleanup: %s, healthcheck: %s)",
				a.cfg.Jobs.CleanupCron, a.cfg.Jobs.HealthCheckCron,
			),
		)
	}
}

func (a *App) serveHTTP(ctx context.Context, e *echo.Echo) error {
	addr := ":" + a.cfg.App.Port
	a.log.Info("app", fmt.Sprintf("starting tidefly on %s (env: %s)", addr, a.cfg.App.Env))
	a.log.Info("app", fmt.Sprintf("OpenAPI docs: http://localhost%s/docs", addr))

	srv := &http.Server{
		Addr:    addr,
		Handler: e,
	}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		a.log.Error("app", "server stopped", err)
		return err
	}
	return nil
}

func (a *App) buildEcho() *echo.Echo {
	e := echo.New()
	e.Use(middleware.Recover(a.log))
	e.Use(middleware.RequestID())
	e.Use(middleware.CORS())
	e.Use(middleware.SecurityHeaders())
	e.Use(
		middleware.RequestLogger(
			a.log, middleware.RequestLoggerOptions{
				SlowThreshold: time.Duration(a.cfg.Logger.SlowQueryMS) * time.Millisecond,
			},
		),
	)

	humaConfig := huma.DefaultConfig("Tidefly API", "0.1.0")
	humaConfig.Info.Description = "Container Management Platform"
	humaAPI := adapter.NewEchoV5Adapter(e, humaConfig)

	v1.Register(
		humaAPI, e, a.ab, a.rt, a.db, a.log,
		a.templateLd, a.notifSvc, a.gitSvc, a.webhookSvc,
		a.asynq, &a.cfg.Traefik, a.notifierSvc,
	)
	return e
}
