package bootstrap

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/database"

	"github.com/google/wire"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/domain/auth"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/git"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/notification"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/template"
	"github.com/tidefly-oss/tidefly-plane/internal/domain/webhook"
	agentsvc "github.com/tidefly-oss/tidefly-plane/internal/infrastructure/agent"
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/infrastructure/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/ingress"
	caddyingress "github.com/tidefly-oss/tidefly-plane/internal/infrastructure/ingress/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/jobs"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/redis"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime/docker"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime/podman"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/ca"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/config"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/crypto"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/metrics"
)

var ProviderSet = wire.NewSet(
	ProvideConfig,
	ProvideLogger,
	ProvideDatabase,
	ProvideRedis,
	ProvideAsynqClient,
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
	return log, func() { log.Sync() }, nil
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

func ProvideRedis(cfg *config.Config) (*goredis.Client, func(), error) {
	client, err := redis.Connect(cfg.Redis.URL)
	if err != nil {
		return nil, nil, err
	}
	return client, func() { _ = client.Close() }, nil
}

func ProvideAsynqClient(cfg *config.Config) (*asynq.Client, func(), error) {
	client := asynq.NewClient(asynq.RedisClientOpt{
		Addr:     cfg.Redis.Addr,
		Username: cfg.Redis.User,
		Password: cfg.Redis.Password,
	})
	return client, func() { _ = client.Close() }, nil
}

func ProvideRuntime(cfg *config.Config, db *gorm.DB) (runtime.Runtime, error) {
	switch runtime.RuntimeType(cfg.Runtime.Type) {
	case runtime.RuntimeDocker:
		return docker.New(cfg.Runtime.SocketPath, db)
	case runtime.RuntimePodman:
		return podman.New(cfg.Runtime.SocketPath, db)
	default:
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

func ProvideJWTService(cfg *config.Config) *auth.Service {
	return auth.New(cfg.Auth.JWTSecret)
}

func ProvideTokenStore(rc *goredis.Client) *auth.TokenStore {
	return auth.NewTokenStore(rc)
}

func ProvideCaddyClient(cfg *config.Config) *caddysvc.Client {
	if !cfg.Caddy.Enabled {
		return nil
	}
	return caddysvc.New(cfg.Caddy)
}

// ProvideCaddyIngress creates the Caddy ingress adapter.
// Returns a no-op adapter if Caddy is disabled so jobs don't panic.
func ProvideCaddyIngress(caddy *caddysvc.Client) ingress.Adapter {
	if caddy == nil {
		return &noopIngressAdapter{}
	}
	return caddyingress.New(caddy)
}

func ProvideTemplateLoader(cfg *config.Config) (*template.Loader, error) {
	return template.NewLoader(cfg.Templates.Dir, cfg.Templates.RepoURL)
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

func ProvideJobServer(
	cfg *config.Config,
	rt runtime.Runtime,
	db *gorm.DB,
	log *applogger.Logger,
	notifSvc *notification.Service,
	metricsReg *metrics.Registry,
	ingressAdapter ingress.Adapter,
) (*jobs.Server, func(), error) {
	if !cfg.Jobs.Enabled || cfg.Redis.URL == "" {
		return nil, func() {}, nil
	}
	srv, err := jobs.NewServer(cfg.Redis, cfg.Jobs, rt, db, log, notifSvc, metricsReg, ingressAdapter)
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

func ProvideAgentServer(cfg *config.Config, db *gorm.DB, caService *ca.Service) *agentsvc.Server {
	return agentsvc.NewServer(db, caService, ":"+cfg.App.AgentGRPCPort)
}

func ProvideAgentClient(srv *agentsvc.Server) *agentsvc.Client {
	return agentsvc.NewClient(srv.Registry())
}

// ── noopIngressAdapter ────────────────────────────────────────────────────────
// Used when Caddy is disabled — all route operations are silently ignored.

type noopIngressAdapter struct{}

func (n *noopIngressAdapter) AddRoute(_ context.Context, _ ingress.Route) error       { return nil }
func (n *noopIngressAdapter) RemoveRoute(_ context.Context, _ string) error           { return nil }
func (n *noopIngressAdapter) UpdateRoute(_ context.Context, _ ingress.Route) error    { return nil }
func (n *noopIngressAdapter) AddTCPRoute(_ context.Context, _ ingress.TCPRoute) error { return nil }
func (n *noopIngressAdapter) RemoveTCPRoute(_ context.Context, _ string) error        { return nil }
