package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/hibiken/asynq"
	"github.com/labstack/echo/v5"
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/infrastructure/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/ingress"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/jobs"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/ca"
	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/metrics"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/api/adapter"
	"github.com/tidefly-oss/tidefly-plane/internal/api/middleware"
	v1 "github.com/tidefly-oss/tidefly-plane/internal/api/v1"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/auth"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/git"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/notification"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/template"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/webhook"
	agentsvc "github.com/tidefly-oss/tidefly-plane/internal/infrastructure/agent"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/logwatcher"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/config"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
)

type App struct {
	cfg         *config.Config
	log         *applogger.Logger
	rt          runtime.Runtime
	db          *gorm.DB
	jwtSvc      *auth.Service
	tokenStore  *auth.TokenStore
	caddy       *caddysvc.Client
	templateLd  *template.Loader
	gitSvc      *git.Service
	webhookSvc  *webhook.Service
	jobServer   *jobs.Server
	asynq       *asynq.Client
	notifSvc    *notification.Service
	notifierSvc *notification.Notifier
	metrics     *metrics.Registry
	caService   *ca.Service
	agentSrv    *agentsvc.Server
	bus         *eventbus.Bus
	ingress     ingress.Adapter
}

func NewApp(
	cfg *config.Config,
	log *applogger.Logger,
	rt runtime.Runtime,
	db *gorm.DB,
	jwtSvc *auth.Service,
	tokenStore *auth.TokenStore,
	caddy *caddysvc.Client,
	templateLd *template.Loader,
	notifSvc *notification.Service,
	gitSvc *git.Service,
	webhookSvc *webhook.Service,
	jobServer *jobs.Server,
	asynqClient *asynq.Client,
	notifierSvc *notification.Notifier,
	metricsReg *metrics.Registry,
	caService *ca.Service,
	agentSrv *agentsvc.Server,
	bus *eventbus.Bus,
	ingressAdapter ingress.Adapter,
) *App {
	return &App{
		cfg: cfg, log: log, rt: rt, db: db,
		jwtSvc: jwtSvc, tokenStore: tokenStore,
		caddy:       caddy,
		templateLd:  templateLd,
		notifSvc:    notifSvc,
		gitSvc:      gitSvc,
		webhookSvc:  webhookSvc,
		jobServer:   jobServer,
		asynq:       asynqClient,
		notifierSvc: notifierSvc,
		metrics:     metricsReg,
		caService:   caService,
		agentSrv:    agentSrv,
		bus:         bus,
		ingress:     ingressAdapter,
	}
}

func (a *App) Run(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)

	if a.cfg.Caddy.Enabled {
		if err := a.caddy.Bootstrap(ctx); err != nil {
			a.log.Warn("app", "caddy bootstrap failed — continuing without proxy", err)
		} else {
			a.log.Info("app", "caddy bootstrapped successfully")
		}
		if err := a.caddy.RegisterDashboard(ctx); err != nil {
			a.log.Warn("app", "dashboard route registration failed", err)
		} else {
			a.log.Info("app", fmt.Sprintf("dashboard: https://dashboard.%s", a.cfg.Caddy.BaseDomain))
			a.log.Info("app", fmt.Sprintf("api: https://tidefly.%s", a.cfg.Caddy.BaseDomain))
		}
	}

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
		a.log.Info("app", fmt.Sprintf(
			"background jobs enabled (cleanup: %s, healthcheck: %s)",
			a.cfg.Jobs.CleanupCron, a.cfg.Jobs.HealthCheckCron,
		))
	}
	a.caService.StartRenewalJob(ctx)
	a.log.Info("app", "CA certificate renewal job started")

	eg.Go(func() error { return a.agentSrv.Start(ctx) })
	a.log.Info("app", fmt.Sprintf("agent gRPC server listening on :%s", a.cfg.App.AgentGRPCPort))

	a.bus.StartRuntimeWatcher(ctx, a.rt, a.log)
	a.bus.StartMetricsTicker(ctx, a.metrics)
	a.log.Info("app", "WebSocket event bus started")
}

func (a *App) serveHTTP(ctx context.Context, e *echo.Echo) error {
	addr := ":" + a.cfg.App.Port
	a.log.Info("app", fmt.Sprintf("starting tidefly-plane on %s (env: %s)", addr, a.cfg.App.Env))
	a.log.Info("app", fmt.Sprintf("OpenAPI docs: http://localhost%s/docs", addr))

	srv := &http.Server{Addr: addr, Handler: e}
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
	e.Use(middleware.GuardDocs(a.db))
	e.Use(middleware.RequestLogger(
		a.log, middleware.RequestLoggerOptions{
			SlowThreshold: time.Duration(a.cfg.Logger.SlowQueryMS) * time.Millisecond,
		},
	))

	humaConfig := huma.DefaultConfig("Tidefly API", "0.0.1-alpha.1")
	humaConfig.Info.Description = "Container Management Platform"
	humaConfig.DocsRenderer = huma.DocsRendererScalar
	humaConfig.Tags = []*huma.Tag{
		{Name: "Admin", Description: "Admin operations"},
		{Name: "Agent", Description: "Worker agent management"},
		{Name: "Auth", Description: "Authentication & sessions"},
		{Name: "Backup", Description: "Backup management"},
		{Name: "Containers", Description: "Container lifecycle"},
		{Name: "Git", Description: "Git integrations"},
		{Name: "Images", Description: "Container images"},
		{Name: "Logs", Description: "Application & audit logs"},
		{Name: "Networks", Description: "Docker networks"},
		{Name: "Notifications", Description: "Notification management"},
		{Name: "Projects", Description: "Project management"},
		{Name: "System", Description: "System health & metrics"},
		{Name: "Templates", Description: "Service templates"},
		{Name: "Volumes", Description: "Docker volumes"},
		{Name: "Webhooks", Description: "Webhook configuration"},
		{Name: "Services", Description: "Manifest-based service management"},
	}

	humaAPI := adapter.NewEchoV5Adapter(e, humaConfig)
	humaAPI.UseMiddleware(middleware.InjectHumaContext())

	v1.Register(
		humaAPI, e, a.jwtSvc, a.tokenStore, a.caddy, a.rt, a.db, a.log,
		a.templateLd, a.notifSvc, a.gitSvc, a.webhookSvc,
		a.asynq, a.notifierSvc, a.metrics,
		a.caService,
		agentsvc.NewClient(a.agentSrv.Registry()),
		a.bus,
		a.ingress,
	)

	return e
}
