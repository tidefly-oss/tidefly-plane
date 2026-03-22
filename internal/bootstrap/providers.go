package bootstrap

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"

	"github.com/google/wire"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/auth"
	"github.com/tidefly-oss/tidefly-backend/internal/config"
	"github.com/tidefly-oss/tidefly-backend/internal/db"
	"github.com/tidefly-oss/tidefly-backend/internal/jobs"
	applogger "github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/metrics"
	"github.com/tidefly-oss/tidefly-backend/internal/redis"
	caddysvc "github.com/tidefly-oss/tidefly-backend/internal/services/caddy"
	"github.com/tidefly-oss/tidefly-backend/internal/services/git"
	"github.com/tidefly-oss/tidefly-backend/internal/services/notifications"
	notifiersvc "github.com/tidefly-oss/tidefly-backend/internal/services/notifier"
	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime/docker"
	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime/podman"
	"github.com/tidefly-oss/tidefly-backend/internal/services/template"
	"github.com/tidefly-oss/tidefly-backend/internal/services/webhook"
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
	ProvideTemplateLoader,
	ProvideNotificationsService,
	ProvideGitService,
	ProvideWebhookService,
	ProvideJobServer,
	ProvideNotifierService,
	ProvideMetricsRegistry,
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
	database, err := db.Connect(cfg.Database.URL, cfg.IsDevelopment())
	if err != nil {
		return nil, nil, err
	}
	database = database.Session(
		&gorm.Session{
			Logger: applogger.NewGORMLogger(cfg.IsDevelopment(), cfg.Logger.SlowQueryMS),
		},
	)
	if err := db.AutoMigrate(database); err != nil {
		return nil, nil, err
	}
	log.SetDB(database)
	return database, func() {}, nil
}

func ProvideRedis(cfg *config.Config) (*goredis.Client, func(), error) {
	client, err := redis.Connect(cfg.Redis.URL)
	if err != nil {
		return nil, nil, err
	}
	return client, func() { _ = client.Close() }, nil
}

func ProvideAsynqClient(cfg *config.Config) (*asynq.Client, func(), error) {
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: cfg.Redis.Addr})
	return client, func() { _ = client.Close() }, nil
}

func ProvideRuntime(cfg *config.Config) (runtime.Runtime, error) {
	switch runtime.RuntimeType(cfg.Runtime.Type) {
	case runtime.RuntimeDocker:
		return docker.New(cfg.Runtime.SocketPath)
	case runtime.RuntimePodman:
		return podman.New(cfg.Runtime.SocketPath)
	default:
		if d, err := docker.New(cfg.Runtime.SocketPath); err == nil {
			if err := d.Ping(context.Background()); err == nil {
				return d, nil
			}
		}
		if p, err := podman.New(cfg.Runtime.SocketPath); err == nil {
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

// ProvideCaddyClient creates the Caddy Admin API client.
// Returns nil if Caddy is disabled — handlers check CaddyEnabled() before use.
func ProvideCaddyClient(cfg *config.Config) *caddysvc.Client {
	if !cfg.Caddy.Enabled {
		return nil
	}
	return caddysvc.New(cfg.Caddy)
}

func ProvideTemplateLoader(cfg *config.Config) (*template.Loader, error) {
	return template.NewLoader(cfg.Templates.Dir)
}

func ProvideNotificationsService(database *gorm.DB) *notifications.Service {
	return notifications.New(database)
}

func ProvideGitService(cfg *config.Config) *git.Service {
	return git.NewService(cfg.App.SecretKey)
}

func ProvideWebhookService(cfg *config.Config) *webhook.Service {
	return webhook.NewService(cfg.App.SecretKey)
}

func ProvideNotifierService(database *gorm.DB, log *applogger.Logger) *notifiersvc.Service {
	return notifiersvc.New(database, log)
}

func ProvideMetricsRegistry() *metrics.Registry {
	return metrics.New()
}

func ProvideJobServer(
	cfg *config.Config,
	rt runtime.Runtime,
	database *gorm.DB,
	log *applogger.Logger,
	notifSvc *notifications.Service,
	metricsReg *metrics.Registry,
) (*jobs.Server, func(), error) {
	if !cfg.Jobs.Enabled || cfg.Redis.URL == "" {
		return nil, func() {}, nil
	}
	srv, err := jobs.NewServer(cfg.Redis, cfg.Jobs, rt, database, log, notifSvc, metricsReg)
	if err != nil {
		return nil, nil, err
	}
	return srv, func() {}, nil
}
