package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App        AppConfig
	Database   DatabaseConfig
	Redis      RedisConfig
	Auth       AuthConfig
	SMTP       SMTPConfig
	Runtime    RuntimeConfig
	Templates  TemplatesConfig
	Logger     LoggerConfig
	LogWatcher LogWatcherConfig
	Jobs       JobsConfig
	Traefik    TraefikConfig
}

type AppConfig struct {
	Env       string
	Port      string
	SecretKey string
}

type DatabaseConfig struct {
	URL string
}

type RedisConfig struct {
	URL      string
	Addr     string
	User     string
	Password string
}

type AuthConfig struct {
	SessionSecret string
	CookieSecret  string
}

// SMTPConfig — user-provided mail server, used for all outgoing mail.
// In dev, point at Mailpit (localhost:1025, no auth).
// In prod, use any SMTP provider (Resend, Postmark, own server, etc.).
type SMTPConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	From     string
	// TLS: "none" | "starttls" | "tls" (default: starttls)
	TLS string
}

type RuntimeConfig struct {
	Type       string
	SocketPath string
}

type TemplatesConfig struct {
	Dir string
}

// TraefikConfig — controls Traefik reverse proxy integration.
type TraefikConfig struct {
	Enabled         bool
	BaseDomain      string
	ACMEEmail       string
	ACMEStaging     bool
	Network         string
	EntrypointHTTP  string
	EntrypointHTTPS string
	ForceHTTPS      bool
}

type LoggerConfig struct {
	Level       string
	DBLevel     string
	SlowQueryMS int64
}

type LogWatcherConfig struct {
	Enabled       bool
	PollInterval  time.Duration
	TailLines     string
	MaxMessageLen int
	DedupWindow   time.Duration
}

type JobsConfig struct {
	Enabled                   bool
	CleanupCron               string
	CleanupOlderThan          time.Duration
	CleanupStoppedContainers  bool
	CleanupDanglingImages     bool
	CleanupUnusedVolumes      bool
	LogRetentionCron          string
	LogRetentionDays          int
	AuditRetentionDays        int
	NotificationRetentionDays int
	MetricsRetentionDays      int
	HealthCheckCron           string
	Concurrency               int
}

func Load() (*Config, error) {
	viper.AutomaticEnv()

	// ── App ──────────────────────────────────────────────────────────────────
	viper.SetDefault("APP_ENV", "development")
	viper.SetDefault("APP_PORT", "8080")

	// ── Redis ─────────────────────────────────────────────────────────────────
	viper.SetDefault("REDIS_ADDR", "127.0.0.1:6379")
	viper.SetDefault("REDIS_USER", "")
	viper.SetDefault("REDIS_PASSWORD", "")

	// ── SMTP ─────────────────────────────────────────────────────────────────
	// Dev default: Mailpit (no auth, no TLS)
	viper.SetDefault("SMTP_HOST", "localhost")
	viper.SetDefault("SMTP_PORT", "1025")
	viper.SetDefault("SMTP_USER", "")
	viper.SetDefault("SMTP_PASSWORD", "")
	viper.SetDefault("SMTP_FROM", "")
	viper.SetDefault("SMTP_TLS", "none") // dev: none, prod: starttls or tls

	// ── Runtime ───────────────────────────────────────────────────────────────
	viper.SetDefault("RUNTIME_TYPE", "")
	viper.SetDefault("RUNTIME_SOCKET", "")
	viper.SetDefault("PODMAN_SOCKET", "/run/user/1000/podman/podman.sock")
	viper.SetDefault("DOCKER_SOCK", "/var/run/docker.sock")

	// ── Templates ─────────────────────────────────────────────────────────────
	viper.SetDefault("TEMPLATES_DIR", "../templates")

	// ── Logger ────────────────────────────────────────────────────────────────
	viper.SetDefault("LOG_LEVEL", "info")
	viper.SetDefault("LOG_DB_LEVEL", "warn")
	viper.SetDefault("LOG_SLOW_QUERY_MS", 500)

	// ── LogWatcher ────────────────────────────────────────────────────────────
	viper.SetDefault("LOGWATCHER_ENABLED", true)
	viper.SetDefault("LOGWATCHER_POLL_INTERVAL", "15s")
	viper.SetDefault("LOGWATCHER_TAIL_LINES", "50")
	viper.SetDefault("LOGWATCHER_MAX_MESSAGE_LEN", 300)
	viper.SetDefault("LOGWATCHER_DEDUP_WINDOW", "2m")

	// ── Jobs ──────────────────────────────────────────────────────────────────
	viper.SetDefault("JOBS_ENABLED", true)
	viper.SetDefault("JOBS_CLEANUP_CRON", "0 3 * * *")
	viper.SetDefault("JOBS_CLEANUP_OLDER_THAN", "24h")
	viper.SetDefault("JOBS_CLEANUP_STOPPED_CONTAINERS", true)
	viper.SetDefault("JOBS_CLEANUP_DANGLING_IMAGES", true)
	viper.SetDefault("JOBS_CLEANUP_UNUSED_VOLUMES", false)
	viper.SetDefault("JOBS_LOG_RETENTION_CRON", "0 4 * * *")
	viper.SetDefault("JOBS_LOG_RETENTION_DAYS", 30)
	viper.SetDefault("JOBS_AUDIT_RETENTION_DAYS", 90)
	viper.SetDefault("JOBS_NOTIFICATION_RETENTION_DAYS", 30)
	viper.SetDefault("JOBS_METRICS_RETENTION_DAYS", 30)
	viper.SetDefault("JOBS_HEALTH_CHECK_CRON", "*/5 * * * *")
	viper.SetDefault("JOBS_CONCURRENCY", 5)

	// ── Traefik ───────────────────────────────────────────────────────────────
	viper.SetDefault("TRAEFIK_ENABLED", false)
	viper.SetDefault("TRAEFIK_BASE_DOMAIN", "")
	viper.SetDefault("TRAEFIK_ACME_EMAIL", "")
	viper.SetDefault("TRAEFIK_ACME_STAGING", false)
	viper.SetDefault("TRAEFIK_NETWORK", "tidefly_internal")
	viper.SetDefault("TRAEFIK_ENTRYPOINT_HTTP", "web")
	viper.SetDefault("TRAEFIK_ENTRYPOINT_HTTPS", "websecure")
	viper.SetDefault("TRAEFIK_FORCE_HTTPS", true)

	// ── Duration parsing ──────────────────────────────────────────────────────
	pollInterval, _ := time.ParseDuration(viper.GetString("LOGWATCHER_POLL_INTERVAL"))
	if pollInterval == 0 {
		pollInterval = 15 * time.Second
	}
	dedupWindow, _ := time.ParseDuration(viper.GetString("LOGWATCHER_DEDUP_WINDOW"))
	if dedupWindow == 0 {
		dedupWindow = 2 * time.Minute
	}
	cleanupOlderThan, _ := time.ParseDuration(viper.GetString("JOBS_CLEANUP_OLDER_THAN"))
	if cleanupOlderThan == 0 {
		cleanupOlderThan = 24 * time.Hour
	}

	cfg := &Config{
		App: AppConfig{
			Env:       viper.GetString("APP_ENV"),
			Port:      viper.GetString("APP_PORT"),
			SecretKey: viper.GetString("APP_SECRET_KEY"),
		},
		Database: DatabaseConfig{
			URL: viper.GetString("DATABASE_URL"),
		},
		Redis: RedisConfig{
			URL:      viper.GetString("REDIS_URL"),
			Addr:     viper.GetString("REDIS_ADDR"),
			User:     viper.GetString("REDIS_USER"),
			Password: viper.GetString("REDIS_PASSWORD"),
		},
		Auth: AuthConfig{
			SessionSecret: viper.GetString("SESSION_SECRET"),
			CookieSecret:  viper.GetString("COOKIE_SECRET"),
		},
		SMTP: SMTPConfig{
			Host:     viper.GetString("SMTP_HOST"),
			Port:     viper.GetString("SMTP_PORT"),
			User:     viper.GetString("SMTP_USER"),
			Password: viper.GetString("SMTP_PASSWORD"),
			From:     viper.GetString("SMTP_FROM"),
			TLS:      viper.GetString("SMTP_TLS"),
		},
		Runtime: RuntimeConfig{
			Type: viper.GetString("RUNTIME_TYPE"),
			SocketPath: resolveRuntimeSocket(
				viper.GetString("RUNTIME_TYPE"),
				viper.GetString("RUNTIME_SOCKET"),
				viper.GetString("PODMAN_SOCKET"),
				viper.GetString("DOCKER_SOCK"),
			),
		},
		Templates: TemplatesConfig{
			Dir: viper.GetString("TEMPLATES_DIR"),
		},
		Logger: LoggerConfig{
			Level:       viper.GetString("LOG_LEVEL"),
			DBLevel:     viper.GetString("LOG_DB_LEVEL"),
			SlowQueryMS: viper.GetInt64("LOG_SLOW_QUERY_MS"),
		},
		LogWatcher: LogWatcherConfig{
			Enabled:       viper.GetBool("LOGWATCHER_ENABLED"),
			PollInterval:  pollInterval,
			TailLines:     viper.GetString("LOGWATCHER_TAIL_LINES"),
			MaxMessageLen: viper.GetInt("LOGWATCHER_MAX_MESSAGE_LEN"),
			DedupWindow:   dedupWindow,
		},
		Jobs: JobsConfig{
			Enabled:                   viper.GetBool("JOBS_ENABLED"),
			CleanupCron:               viper.GetString("JOBS_CLEANUP_CRON"),
			CleanupOlderThan:          cleanupOlderThan,
			CleanupStoppedContainers:  viper.GetBool("JOBS_CLEANUP_STOPPED_CONTAINERS"),
			CleanupDanglingImages:     viper.GetBool("JOBS_CLEANUP_DANGLING_IMAGES"),
			CleanupUnusedVolumes:      viper.GetBool("JOBS_CLEANUP_UNUSED_VOLUMES"),
			LogRetentionCron:          viper.GetString("JOBS_LOG_RETENTION_CRON"),
			LogRetentionDays:          viper.GetInt("JOBS_LOG_RETENTION_DAYS"),
			AuditRetentionDays:        viper.GetInt("JOBS_AUDIT_RETENTION_DAYS"),
			NotificationRetentionDays: viper.GetInt("JOBS_NOTIFICATION_RETENTION_DAYS"),
			MetricsRetentionDays:      viper.GetInt("JOBS_METRICS_RETENTION_DAYS"),
			HealthCheckCron:           viper.GetString("JOBS_HEALTH_CHECK_CRON"),
			Concurrency:               viper.GetInt("JOBS_CONCURRENCY"),
		},
		Traefik: TraefikConfig{
			Enabled:         viper.GetBool("TRAEFIK_ENABLED"),
			BaseDomain:      viper.GetString("TRAEFIK_BASE_DOMAIN"),
			ACMEEmail:       viper.GetString("TRAEFIK_ACME_EMAIL"),
			ACMEStaging:     viper.GetBool("TRAEFIK_ACME_STAGING"),
			Network:         viper.GetString("TRAEFIK_NETWORK"),
			EntrypointHTTP:  viper.GetString("TRAEFIK_ENTRYPOINT_HTTP"),
			EntrypointHTTPS: viper.GetString("TRAEFIK_ENTRYPOINT_HTTPS"),
			ForceHTTPS:      viper.GetBool("TRAEFIK_FORCE_HTTPS"),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks that all required fields are set and cross-validates
// dependent settings (e.g. Traefik requires BaseDomain + ACMEEmail in prod).
func (c *Config) Validate() error {
	var errs []string

	// ── Hard requirements — app cannot start without these ───────────────────
	if c.App.SecretKey == "" {
		errs = append(errs, "APP_SECRET_KEY is required (min 32 chars)")
	} else if len(c.App.SecretKey) < 32 {
		errs = append(errs, fmt.Sprintf("APP_SECRET_KEY too short (%d chars, need 32)", len(c.App.SecretKey)))
	}
	if c.Database.URL == "" {
		errs = append(errs, "DATABASE_URL is required")
	}
	if c.Auth.SessionSecret == "" {
		errs = append(errs, "SESSION_SECRET is required")
	}
	if c.Auth.CookieSecret == "" {
		errs = append(errs, "COOKIE_SECRET is required")
	}

	// ── Runtime socket ────────────────────────────────────────────────────────
	if c.Runtime.SocketPath == "" {
		errs = append(errs, "DOCKER_SOCK (or RUNTIME_SOCKET / PODMAN_SOCKET) is required")
	}

	// ── Traefik cross-validation ──────────────────────────────────────────────
	if c.Traefik.Enabled {
		if c.Traefik.BaseDomain == "" {
			errs = append(errs, "TRAEFIK_BASE_DOMAIN is required when TRAEFIK_ENABLED=true")
		}
		if c.Traefik.ACMEEmail == "" && !c.IsDevelopment() {
			errs = append(errs, "TRAEFIK_ACME_EMAIL is required when TRAEFIK_ENABLED=true in production")
		}
	}

	// ── SMTP prod cross-validation ────────────────────────────────────────────
	// In prod we warn when SMTP looks like the Mailpit default — mails won't
	// reach anyone. We don't hard-fail because the app is usable without mail.
	if !c.IsDevelopment() {
		if c.SMTP.Host == "localhost" || c.SMTP.Host == "127.0.0.1" {
			errs = append(
				errs,
				"SMTP_HOST appears to be a local dev server (Mailpit?) — "+
					"set a real SMTP host for production or mails will not be delivered",
			)
		}
	}

	if len(errs) > 0 {
		return errors.New("config validation failed:\n  - " + strings.Join(errs, "\n  - "))
	}
	return nil
}

// SMTPConfigured returns true when a real SMTP server is configured.
// Use this to skip sending mail in dev without a provider.
func (c *Config) SMTPConfigured() bool {
	return c.SMTP.Host != "" &&
		c.SMTP.Host != "localhost" &&
		c.SMTP.Host != "127.0.0.1"
}

// RedisConfigured returns true when a Redis URL or addr is set.
// Jobs and LogWatcher disable themselves when this returns false.
func (c *Config) RedisConfigured() bool {
	return c.Redis.URL != "" || c.Redis.Addr != ""
}

func (c *Config) IsDevelopment() bool {
	return c.App.Env == "development"
}

func resolveRuntimeSocket(runtimeType, runtimeSocket, podmanSocket, dockerSocket string) string {
	if runtimeSocket != "" {
		return runtimeSocket
	}
	switch runtimeType {
	case "podman":
		return podmanSocket
	case "docker":
		return dockerSocket
	default:
		return dockerSocket
	}
}
