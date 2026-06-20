package config

import "github.com/spf13/viper"

func setAppDefaults() {
	viper.SetDefault("APP_ENV", "development")
	viper.SetDefault("APP_PORT", "8181")
	viper.SetDefault("APP_SECRET_KEY", "")
	viper.SetDefault("API_DOCS_ENABLED", false)
	viper.SetDefault("TIDEFLY_ENCRYPTION_KEY", "")
	viper.SetDefault("AGENT_GRPC_PORT", 7443)
}

func setDatabaseDefaults() {
	viper.SetDefault("DATABASE_URL", "")
	viper.SetDefault("POSTGRES_USER", "tidefly-plane")
	viper.SetDefault("POSTGRES_PASSWORD", "")
	viper.SetDefault("POSTGRES_DB", "tidefly-plane")
}

func setRedisDefaults() {
	viper.SetDefault("REDIS_URL", "")
	viper.SetDefault("REDIS_ADDR", "127.0.0.1:6379")
	viper.SetDefault("REDIS_USER", "tidefly-plane")
	viper.SetDefault("REDIS_PASSWORD", "")
}

func setRuntimeDefaults() {
	viper.SetDefault("RUNTIME_TYPE", "docker")
	viper.SetDefault("RUNTIME_SOCKET", "")
	viper.SetDefault("DOCKER_SOCK", "/var/run/docker.sock")
	viper.SetDefault("PODMAN_SOCK", "/run/user/1000/podman/podman.sock")
}

func setCaddyDefaults() {
	viper.SetDefault("CADDY_ENABLED", true)
	viper.SetDefault("CADDY_ADMIN_URL", "http://caddy:2019")
	viper.SetDefault("CADDY_BASE_DOMAIN", "")
	viper.SetDefault("CADDY_ACME_EMAIL", "")
	viper.SetDefault("CADDY_ACME_STAGING", false)
	viper.SetDefault("CADDY_FORCE_HTTPS", true)
	viper.SetDefault("CADDY_INTERNAL_TLS", true)
}

func setSMTPDefaults() {
	viper.SetDefault("SMTP_HOST", "localhost")
	viper.SetDefault("SMTP_PORT", "1025")
	viper.SetDefault("SMTP_USER", "")
	viper.SetDefault("SMTP_PASSWORD", "")
	viper.SetDefault("SMTP_FROM", "")
	viper.SetDefault("SMTP_TLS", "none")
}

func setLoggingDefaults() {
	viper.SetDefault("LOG_LEVEL", "info")
	viper.SetDefault("LOG_DB_LEVEL", "warn")
	viper.SetDefault("LOG_SLOW_QUERY_MS", 500)
}

func setLogwatcherDefaults() {
	viper.SetDefault("LOGWATCHER_ENABLED", true)
	viper.SetDefault("LOGWATCHER_POLL_INTERVAL", "15s")
	viper.SetDefault("LOGWATCHER_TAIL_LINES", "50")
	viper.SetDefault("LOGWATCHER_MAX_MESSAGE_LEN", 300)
	viper.SetDefault("LOGWATCHER_DEDUP_WINDOW", "2m")
}

func setJobsDefaults() {
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
}

func setTemplatesDefaults() {
	viper.SetDefault("TEMPLATES_DIR", "/etc/tidefly-plane/templates")
	viper.SetDefault("TEMPLATES_REPO", "https://github.com/tidefly-oss/tidefly-templates")
}
