package config

import (
	"time"

	"github.com/spf13/viper"
)

func ParseConfig() *Config {
	port := viper.GetString("APP_PORT")
	if !isPortAvailable(port) {
		port = "8989"
	}

	// ── Duration parsing ────────────────────────────────────────────────
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
			Env:         viper.GetString("APP_ENV"),
			Port:        port,
			SecretKey:   viper.GetString("APP_SECRET_KEY"),
			DocsEnabled: viper.GetBool("API_DOCS_ENABLED"),
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
			Enabled:          viper.GetBool("TRAEFIK_ENABLED"),
			BaseDomain:       viper.GetString("TRAEFIK_BASE_DOMAIN"),
			ACMEEmail:        viper.GetString("TRAEFIK_ACME_EMAIL"),
			ACMEStaging:      viper.GetBool("TRAEFIK_ACME_STAGING"),
			Network:          viper.GetString("TRAEFIK_NETWORK"),
			EntrypointHTTP:   viper.GetString("TRAEFIK_ENTRYPOINT_HTTP"),
			EntrypointHTTPS:  viper.GetString("TRAEFIK_ENTRYPOINT_HTTPS"),
			ForceHTTPS:       viper.GetBool("TRAEFIK_FORCE_HTTPS"),
			DashboardEnabled: viper.GetBool("TRAEFIK_DASHBOARD_ENABLED"),
		},
	}

	return cfg
}
