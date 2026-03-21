package config

import "time"

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
	Caddy      CaddyConfig
}

type AppConfig struct {
	Env         string
	Port        string
	SecretKey   string
	DocsEnabled bool
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
	JWTSecret string
}

type SMTPConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	From     string
	TLS      string
}

type RuntimeConfig struct {
	Type       string
	SocketPath string
}

type TemplatesConfig struct {
	Dir string
}

// CaddyConfig controls Caddy reverse proxy integration.
// All configuration is applied via the Caddy Admin API — no Caddyfile needed.
type CaddyConfig struct {
	Enabled     bool
	AdminURL    string
	BaseDomain  string
	ACMEEmail   string
	ACMEStaging bool
	ForceHTTPS  bool
	InternalTLS bool
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
