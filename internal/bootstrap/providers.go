package bootstrap

import (
	"context"
	"fmt"

	"github.com/google/wire"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/tidefly-oss/tidefly-plane/internal/reconciler"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/agent"
	"github.com/tidefly-oss/tidefly-plane/internal/auth"
	"github.com/tidefly-oss/tidefly-plane/internal/git"
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/infra/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/database"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/ingress"
	caddyingress "github.com/tidefly-oss/tidefly-plane/internal/infra/ingress/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime/docker"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime/podman"
	"github.com/tidefly-oss/tidefly-plane/internal/jobs"
	"github.com/tidefly-oss/tidefly-plane/internal/notification"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/ca"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/config"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/crypto"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/metrics"
	"github.com/tidefly-oss/tidefly-plane/internal/template"
	"github.com/tidefly-oss/tidefly-plane/internal/webhook"
)

var ProviderSet = wire.NewSet(
	ProvideConfig,
	ProvideLogger,
	ProvideDatabase,
	ProvidePgxPool,
	ProvideRuntime,
	ProvideJWTService,
	ProvideTokenStore,
	ProvideCaddyClient,
	ProvideCaddyIngress,
	ProvideTemplateLoader,
	ProvideNotificationService,
	ProvideNotifier,
	ProvideGitService,
	ProvideWebhookService,
	ProvideReconciler,
	ProvideJobServer,
	ProvideMetricsRegistry,
	ProvideEventBus,
	ProvideCAService,
	ProvideAgentServer,
	ProvideAgentClient,
	NewApp,
)

func ProvideConfig() (*config.Config, error) {
	return config.Load()
}

func ProvideLogger(cfg *config.Config) (*applogger.Logger, func(), error) {
	dbLogLevel := applogger.DBLogWarnAndAbove
	switch cfg.Logger.DBLevel {
	case "error":
		dbLogLevel = applogger.DBLogErrorAndAbove
	case "none":
		dbLogLevel = applogger.DBLogNone
	}
	log, err := applogger.New(cfg.IsDevelopment(), nil, applogger.WithDBLogLevel(dbLogLevel))
	if err != nil {
		return nil, nil, err
	}
	return log, func() {}, nil
}

func ProvideDatabase(cfg *config.Config, log *applogger.Logger) (*gorm.DB, func(), error) {
	db, err := database.Connect(cfg.Database.URL, cfg.IsDevelopment())
	if err != nil {
		return nil, nil, err
	}
	if err := database.AutoMigrate(db); err != nil {
		return nil, nil, err
	}
	db = db.Session(&gorm.Session{
		Logger: applogger.NewGORMLogger(cfg.IsDevelopment(), cfg.Logger.SlowQueryMS),
	})
	log.SetDB(db)
	return db, func() {}, nil
}

// ProvidePgxPool creates a pgxpool from the same DATABASE_URL.
// River uses pgx directly (not GORM) for its job queue tables.
func ProvidePgxPool(cfg *config.Config) (*pgxpool.Pool, func(), error) {
	pool, err := pgxpool.New(context.Background(), cfg.Database.URL)
	if err != nil {
		return nil, nil, fmt.Errorf("pgxpool: %w", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("pgxpool: ping failed: %w", err)
	}
	return pool, func() { pool.Close() }, nil
}

func ProvideRuntime(cfg *config.Config, db *gorm.DB) (runtime.Runtime, error) {
	switch runtime.RuntimeType(cfg.Runtime.Type) {
	case runtime.RuntimeDocker:
		return docker.New(cfg.Runtime.SocketPath, db)
	case runtime.RuntimePodman:
		return podman.New(cfg.Runtime.SocketPath, db)
	default:
		// Auto-detect: try Docker first, then Podman
		if d, err := docker.New(cfg.Runtime.SocketPath, db); err == nil {
			if err := d.Ping(context.Background()); err == nil {
				return d, nil
			}
		}
		if p, err := podman.New(cfg.Runtime.SocketPath, db); err == nil {
			if err := p.Ping(context.Background()); err == nil {
				return p, nil
			}
		}
		return nil, fmt.Errorf("no container runtime found: neither docker nor podman is reachable")
	}
}

func ProvideJWTService(cfg *config.Config) *auth.JWTService {
	return auth.NewJWTService(cfg.Auth.JWTSecret)
}

// ProvideTokenStore now uses the DB instead of Redis for token storage.
// auth.TokenStore must accept *gorm.DB — update that struct if needed.
func ProvideTokenStore(db *gorm.DB) *auth.TokenStore {
	return auth.NewTokenStore(db)
}

func ProvideCaddyClient(cfg *config.Config) *caddysvc.Client {
	if !cfg.Caddy.Enabled {
		return nil
	}
	return caddysvc.New(cfg.Caddy)
}

func ProvideCaddyIngress(caddy *caddysvc.Client) ingress.Adapter {
	if caddy == nil {
		return &noopIngressAdapter{}
	}
	return caddyingress.New(caddy)
}

func ProvideTemplateLoader() *template.Loader {
	return template.NewLoader()
}

func ProvideEventBus() *eventbus.Bus {
	return eventbus.New()
}

func ProvideNotificationService(db *gorm.DB, bus *eventbus.Bus) *notification.Service {
	return notification.New(db, bus)
}

func ProvideNotifier(db *gorm.DB, log *applogger.Logger) *notification.Notifier {
	return notification.NewNotifier(db, log)
}

func ProvideGitService(cfg *config.Config) *git.Service {
	return git.NewService(cfg.App.SecretKey)
}

func ProvideWebhookService(cfg *config.Config) *webhook.Service {
	return webhook.NewService(cfg.App.SecretKey)
}

func ProvideMetricsRegistry() *metrics.Registry {
	return metrics.New()
}

func ProvideReconciler(
	db *gorm.DB,
	rt runtime.Runtime,
	ing ingress.Adapter,
	notifSvc *notification.Service,
	log *applogger.Logger,
) *reconciler.Reconciler {
	return reconciler.New(db, rt, ing, notifSvc, log)
}

func ProvideJobServer(
	cfg *config.Config,
	pool *pgxpool.Pool,
	rt runtime.Runtime,
	db *gorm.DB,
	log *applogger.Logger,
	notifSvc *notification.Service,
	notifier *notification.Notifier,
	metricsReg *metrics.Registry,
	ingressAdapter ingress.Adapter,
	agentClient *agent.Client,
	rec *reconciler.Reconciler,
	bus *eventbus.Bus,
) (*jobs.Server, func(), error) {
	if !cfg.Jobs.Enabled {
		return nil, func() {}, nil
	}

	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("river migrate: acquire conn: %w", err)
	}
	defer conn.Release()

	migrator, err := rivermigrate.New(riverpgxv5.New(pool), &rivermigrate.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("river migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, &rivermigrate.MigrateOpts{}); err != nil {
		return nil, nil, fmt.Errorf("river migrate: %w", err)
	}

	srv, err := jobs.NewServer(pool, cfg.Jobs, rt, db, log, notifSvc, notifier, metricsReg, ingressAdapter, agentClient, rec, bus)
	if err != nil {
		return nil, nil, err
	}
	return srv, func() {}, nil
}

func ProvideCAService(cfg *config.Config, db *gorm.DB) (*ca.Service, error) {
	encKey, err := crypto.KeyFromBase64(cfg.App.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("ca: invalid encryption key: %w", err)
	}
	svc := ca.New(db, encKey)
	if err := svc.Init(); err != nil {
		return nil, fmt.Errorf("ca: init failed: %w", err)
	}
	return svc, nil
}

func ProvideAgentServer(cfg *config.Config, db *gorm.DB, caService *ca.Service, bus *eventbus.Bus) *agent.Server {
	return agent.NewServer(db, caService, bus, ":"+cfg.App.AgentGRPCPort)
}

func ProvideAgentClient(srv *agent.Server) *agent.Client {
	return agent.NewClient(srv.Registry())
}

// ── noopIngressAdapter ────────────────────────────────────────────────────────

type noopIngressAdapter struct{}

func (n *noopIngressAdapter) AddRoute(_ context.Context, _ ingress.Route) error       { return nil }
func (n *noopIngressAdapter) RemoveRoute(_ context.Context, _ string) error           { return nil }
func (n *noopIngressAdapter) UpdateRoute(_ context.Context, _ ingress.Route) error    { return nil }
func (n *noopIngressAdapter) AddTCPRoute(_ context.Context, _ ingress.TCPRoute) error { return nil }
func (n *noopIngressAdapter) RemoveTCPRoute(_ context.Context, _ string) error        { return nil }
