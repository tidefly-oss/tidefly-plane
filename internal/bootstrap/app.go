package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/tidefly-oss/tidefly-plane/internal/agent"
	"github.com/tidefly-oss/tidefly-plane/internal/auth"
	"github.com/tidefly-oss/tidefly-plane/internal/git"
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/infra/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/ingress"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/jobs"
	"github.com/tidefly-oss/tidefly-plane/internal/logmon"
	middleware2 "github.com/tidefly-oss/tidefly-plane/internal/middleware"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/notification"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_ca"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/_eventbus"
	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/_logger"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/config"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/metrics"
	"github.com/tidefly-oss/tidefly-plane/internal/system"
	"github.com/tidefly-oss/tidefly-plane/internal/template"
	"github.com/tidefly-oss/tidefly-plane/internal/webhook"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

type App struct {
	cfg         *config.Config
	log         *applogger.Logger
	rt          runtime.Runtime
	db          *gorm.DB
	jwtSvc      *auth.JWTService
	tokenStore  *auth.TokenStore
	caddy       *caddysvc.Client
	templateLd  *template.Loader
	gitSvc      *git.Service
	webhookSvc  *webhook.Service
	jobServer   *jobs.Server
	riverClient *river.Client[pgx.Tx]
	notifSvc    *notification.Service
	notifierSvc *notification.Notifier
	metrics     *metrics.Registry
	caService   *_ca.Service
	agentSrv    *agent.Server
	bus         *_eventbus.Bus
	ingress     ingress.Adapter
}

func NewApp(
	cfg *config.Config,
	log *applogger.Logger,
	rt runtime.Runtime,
	db *gorm.DB,
	jwtSvc *auth.JWTService,
	tokenStore *auth.TokenStore,
	caddy *caddysvc.Client,
	templateLd *template.Loader,
	notifSvc *notification.Service,
	gitSvc *git.Service,
	webhookSvc *webhook.Service,
	jobServer *jobs.Server,
	notifierSvc *notification.Notifier,
	metricsReg *metrics.Registry,
	caService *_ca.Service,
	agentSrv *agent.Server,
	bus *_eventbus.Bus,
	ingressAdapter ingress.Adapter,
) *App {
	app := &App{
		cfg:         cfg,
		log:         log,
		rt:          rt,
		db:          db,
		jwtSvc:      jwtSvc,
		tokenStore:  tokenStore,
		caddy:       caddy,
		templateLd:  templateLd,
		notifSvc:    notifSvc,
		gitSvc:      gitSvc,
		webhookSvc:  webhookSvc,
		jobServer:   jobServer,
		riverClient: jobServer.Client(),
		notifierSvc: notifierSvc,
		metrics:     metricsReg,
		caService:   caService,
		agentSrv:    agentSrv,
		bus:         bus,
		ingress:     ingressAdapter,
	}

	log.SetNotifier(
		func(ctx context.Context, sourceID, sourceName string, severity models.NotificationSeverity, msg string) error {
			return notifSvc.Upsert(ctx, sourceID, sourceName, severity, msg)
		},
		func(title, message, level string) {
			notifierSvc.Send(context.Background(), notification.Event{
				Title:   title,
				Message: message,
				Level:   level,
			})
		},
	)

	applogger.SetContextEnricher(middleware2.NewEnricher())

	return app
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

	r := a.buildRouter()
	eg.Go(func() error { return a.serveHTTP(ctx, r) })

	return eg.Wait()
}

func (a *App) startBackgroundServices(eg *errgroup.Group, ctx context.Context) {
	if a.cfg.LogWatcher.Enabled {
		watcher := logmon.New(a.rt, a.log, a.cfg.LogWatcher, a.notifSvc, a.notifierSvc)
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

	a.bus.StartMetricsTicker(ctx, a.metrics)
	a.log.Info("app", "WebSocket event bus started")

	system.NewHandler(a.rt, a.log, a.metrics, a.bus).StartVersionRefresh(ctx)
	a.log.Info("app", "version refresh started (interval: 20m)")
}

func (a *App) serveHTTP(ctx context.Context, r http.Handler) error {
	addr := ":" + a.cfg.App.Port
	a.log.Info("app", fmt.Sprintf("starting tidefly-plane on %s (env: %s)", addr, a.cfg.App.Env))
	a.log.Info("app", fmt.Sprintf("OpenAPI docs: http://localhost%s/docs", addr))

	srv := &http.Server{
		Addr:           addr,
		Handler:        r,
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   60 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20,
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

func (a *App) buildRouter() chi.Router {
	r := chi.NewRouter()

	r.Use(middleware2.Recover(a.log))
	r.Use(middleware2.RateLimitAPI())
	r.Use(middleware2.RequestID())
	r.Use(middleware2.CORS())
	r.Use(middleware2.SecurityHeaders())
	r.Use(middleware2.GuardDocs(a.db))
	r.Use(middleware2.RequestLogger(
		a.log, middleware2.RequestLoggerOptions{
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

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	humaAPI := humachi.New(r, humaConfig)
	humaAPI.UseMiddleware(middleware2.InjectHumaContext())

	Register(
		humaAPI, r, a.jwtSvc, a.tokenStore, a.caddy, a.rt, a.db, a.log,
		a.templateLd, a.notifSvc, a.gitSvc, a.webhookSvc,
		a.riverClient, a.notifierSvc, a.metrics,
		a.caService,
		agent.NewClient(a.agentSrv.Registry()),
		a.bus,
		a.ingress,
	)

	return r
}
