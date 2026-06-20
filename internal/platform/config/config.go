package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

func Load() (*Config, error) {
	viper.AutomaticEnv()
	setAppDefaults()
	setDatabaseDefaults()
	setJobsDefaults()
	setLogwatcherDefaults()
	setLoggingDefaults()
	setRedisDefaults()
	setRuntimeDefaults()
	setSMTPDefaults()
	setTemplatesDefaults()
	setCaddyDefaults()
	cfg := parse()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func parse() *Config {
	port := viper.GetString("APP_PORT")
	if !isPortAvailable(port) {
		port = "8989"
	}

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

	return &Config{
		App: AppConfig{
			Env:           viper.GetString("APP_ENV"),
			Port:          port,
			SecretKey:     viper.GetString("APP_SECRET_KEY"),
			DocsEnabled:   viper.GetBool("API_DOCS_ENABLED"),
			EncryptionKey: viper.GetString("TIDEFLY_ENCRYPTION_KEY"),
			AgentGRPCPort: viper.GetString("AGENT_GRPC_PORT"),
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
			JWTSecret: viper.GetString("JWT_SECRET"),
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
			Dir:     viper.GetString("TEMPLATES_DIR"),
			RepoURL: viper.GetString("TEMPLATES_REPO"),
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
			AuditRetentionDays:        viper.GetInt("JOBS_AUDIT_RETENTION_DAYS"),
			NotificationRetentionDays: viper.GetInt("JOBS_NOTIFICATION_RETENTION_DAYS"),
			HealthCheckCron:           viper.GetString("JOBS_HEALTH_CHECK_CRON"),
			Concurrency:               viper.GetInt("JOBS_CONCURRENCY"),
		},
		Caddy: CaddyConfig{
			Enabled:     viper.GetBool("CADDY_ENABLED"),
			AdminURL:    viper.GetString("CADDY_ADMIN_URL"),
			BaseDomain:  viper.GetString("CADDY_BASE_DOMAIN"),
			ACMEEmail:   viper.GetString("CADDY_ACME_EMAIL"),
			ACMEStaging: viper.GetBool("CADDY_ACME_STAGING"),
			ForceHTTPS:  viper.GetBool("CADDY_FORCE_HTTPS"),
			InternalTLS: viper.GetBool("CADDY_INTERNAL_TLS"),
		},
		Prometheus: PrometheusConfig{
			URL: viper.GetString("PROMETHEUS_URL"),
		},
	}
}

func (c *Config) Validate() error {
	var errs []string

	if c.App.SecretKey == "" {
		errs = append(errs, "APP_SECRET_KEY is required (min 32 chars)")
	} else if len(c.App.SecretKey) < 32 {
		errs = append(errs, fmt.Sprintf("APP_SECRET_KEY too short (%d chars, need 32)", len(c.App.SecretKey)))
	}
	if c.Database.URL == "" {
		errs = append(errs, "DATABASE_URL is required")
	}
	if c.Auth.JWTSecret == "" {
		errs = append(errs, "JWT_SECRET is required")
	}
	if c.Runtime.SocketPath == "" {
		errs = append(errs, "DOCKER_SOCK (or RUNTIME_SOCKET / PODMAN_SOCKET) is required")
	}
	if c.Caddy.Enabled {
		if c.Caddy.AdminURL == "" {
			errs = append(errs, "CADDY_ADMIN_URL is required when CADDY_ENABLED=true")
		}
		if c.Caddy.BaseDomain == "" {
			errs = append(errs, "CADDY_BASE_DOMAIN is required when CADDY_ENABLED=true")
		}
		if c.Caddy.ACMEEmail == "" && !c.IsDevelopment() {
			errs = append(errs, "CADDY_ACME_EMAIL is required when CADDY_ENABLED=true in production")
		}
	}
	if !c.IsDevelopment() {
		if c.SMTP.Host == "localhost" || c.SMTP.Host == "127.0.0.1" {
			errs = append(errs, "SMTP_HOST appears to be a local dev server — set a real SMTP host for production")
		}
	}

	if len(errs) > 0 {
		return errors.New("config validation failed:\n  - " + strings.Join(errs, "\n  - "))
	}
	return nil
}
