package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

var ignoredMessages = []string{
	"context canceled",
	"context deadline exceeded",
	"connection reset by peer",
	"broken pipe",
	"EOF",
	"i/o timeout",
	"use of closed network connection",
}

// AuditAction constants for security-relevant events.
type AuditAction string

const (
	AuditLogin          AuditAction = "auth.login"
	AuditLogout         AuditAction = "auth.logout"
	AuditLoginFailed    AuditAction = "auth.login_failed"
	AuditRegister       AuditAction = "auth.register"
	AuditPasswordChange AuditAction = "auth.password_change"

	AuditAdminUserCreate         AuditAction = "admin.user.create"
	AuditAdminUserUpdate         AuditAction = "admin.user.update"
	AuditAdminUserDelete         AuditAction = "admin.user.delete"
	AuditAdminUserPasswordReset  AuditAction = "admin.user.password_reset"
	AuditAdminUserProjectsUpdate AuditAction = "admin.user.projects_update"
	AuditAdminSettingsUpdate     AuditAction = "admin.settings.update"

	AuditContainerDeploy  AuditAction = "container.deploy"
	AuditContainerStart   AuditAction = "container.start"
	AuditContainerStop    AuditAction = "container.stop"
	AuditContainerRestart AuditAction = "container.restart"
	AuditContainerDelete  AuditAction = "container.delete"
	AuditContainerUpdate  AuditAction = "container.update"

	AuditImagePull   AuditAction = "image.pull"
	AuditImageDelete AuditAction = "image.delete"

	AuditGitTokenAdd    AuditAction = "git.token.add"
	AuditGitTokenDelete AuditAction = "git.token.delete"
	AuditGitRepoLink    AuditAction = "git.repo.link"

	AuditProjectCreate AuditAction = "project.create"
	AuditProjectUpdate AuditAction = "project.update"
	AuditProjectDelete AuditAction = "project.delete"

	AuditNetworkDelete AuditAction = "network.delete"
	AuditVolumeDelete  AuditAction = "volume.delete"

	AuditWebhookCreate AuditAction = "webhooks.create"
	AuditWebhookUpdate AuditAction = "webhooks.update"
	AuditWebhookDelete AuditAction = "webhooks.delete"
	AuditWebhookRotate AuditAction = "webhooks.rotate_secret"
)

// DBLogLevel controls what gets written to the AppLog table.
type DBLogLevel int

const (
	DBLogWarnAndAbove DBLogLevel = iota
	DBLogErrorAndAbove
	DBLogNone
)

// Logger wraps slog.Logger and adds DB-backed audit/app-log persistence.
type Logger struct {
	slog       *slog.Logger
	db         *gorm.DB
	dbLogLevel DBLogLevel
}

type Option func(*Logger)

func WithDBLogLevel(level DBLogLevel) Option {
	return func(l *Logger) { l.dbLogLevel = level }
}

// New builds the application logger.
// In development: human-readable text output.
// In production:  JSON output at Warn level and above.
func New(isDev bool, db *gorm.DB, opts ...Option) (*Logger, error) {
	var handler slog.Handler
	if isDev {
		handler = slog.NewTextHandler(
			os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			},
		)
	} else {
		handler = slog.NewJSONHandler(
			os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelWarn,
			},
		)
	}

	l := &Logger{
		slog:       slog.New(handler),
		db:         db,
		dbLogLevel: DBLogWarnAndAbove,
	}
	for _, o := range opts {
		o(l)
	}
	return l, nil
}

// Sync is a no-op for slog (kept for API compatibility with former zap logger).
func (l *Logger) Sync() {}

// SetDB hängt die DB nachträglich ein. Wird von bootstrap.ProvideDatabase
// aufgerufen sobald die DB bereit ist — löst den Zirkelbezug Logger↔DB.
func (l *Logger) SetDB(db *gorm.DB) { l.db = db }

// ── Structured logging helpers ────────────────────────────────────────────────

func (l *Logger) Info(component, msg string, args ...any) {
	l.slog.Info(msg, prepend("component", component, args)...)
}

func (l *Logger) Warn(component, msg string, args ...any) {
	l.slog.Warn(msg, prepend("component", component, args)...)
	if l.dbLogLevel <= DBLogWarnAndAbove {
		l.writeAppLog("WARN", component, msg, "", "")
	}
}

func (l *Logger) Error(component, msg string, err error, args ...any) {
	extra := prepend("component", component, args)
	if err != nil {
		extra = append(extra, slog.String("error", err.Error()))
	}
	l.slog.Error(msg, extra...)
	if l.dbLogLevel <= DBLogErrorAndAbove {
		errStr := ""
		if err != nil {
			errStr = err.Error()
		}
		l.writeAppLog("ERROR", component, msg, errStr, "")
	}
}

func (l *Logger) Errorw(component, msg string, args ...any) {
	l.slog.Error(msg, prepend("component", component, args)...)
	// Extract error string for DB log if "error" key present
	errStr := ""
	for i := 0; i+1 < len(args); i += 2 {
		if k, ok := args[i].(string); ok && k == "error" {
			errStr = fmt.Sprintf("%v", args[i+1])
			break
		}
	}
	if l.dbLogLevel <= DBLogErrorAndAbove {
		l.writeAppLog("ERROR", component, msg, errStr, "")
	}
}

// Also add Warnw for consistency:

func (l *Logger) Warnw(component, msg string, args ...any) {
	l.slog.Warn(msg, prepend("component", component, args)...)
	if l.dbLogLevel <= DBLogWarnAndAbove {
		l.writeAppLog("WARN", component, msg, "", "")
	}
}

func (l *Logger) Fatal(component, msg string, err error) {
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	l.writeAppLog("FATAL", component, msg, errStr, "")
	l.slog.Error(
		msg,
		slog.String("component", component),
		slog.String("error", errStr),
		slog.String("level", "FATAL"),
	)
	os.Exit(1)
}

func (l *Logger) ContainerEvent(level, containerID, containerName, msg, errDetail string) {
	component := "container:" + containerName
	l.slog.Info(
		"container-event",
		slog.String("level", level),
		slog.String("container_id", containerID),
		slog.String("container_name", containerName),
		slog.String("msg", msg),
	)
	l.writeAppLog(level, component, msg, errDetail, containerID)
}

// Slog returns the raw *slog.Logger for libraries that accept it directly.
func (l *Logger) Slog() *slog.Logger { return l.slog }

func (l *Logger) writeAppLog(level, component, msg, errStr, containerID string) {
	if l.db == nil {
		return
	}
	for _, ignore := range ignoredMessages {
		if strings.Contains(msg, ignore) || strings.Contains(errStr, ignore) {
			return
		}
	}
	go func() {
		l.db.Create(&models.AppLog{
			Level:       level,
			Component:   component,
			Message:     msg,
			Error:       errStr,
			ContainerID: containerID,
		})
	}()
}

// ── Audit Logging ─────────────────────────────────────────────────────────────

type AuditEntry struct {
	Action     AuditAction
	UserID     string
	UserEmail  string
	IPAddress  string
	UserAgent  string
	ResourceID string
	Details    string
	Success    bool
}

func (l *Logger) Audit(ctx context.Context, entry AuditEntry) {
	if l.db != nil {
		go func() {
			l.db.Create(
				&models.AuditLog{
					Action:     string(entry.Action),
					UserID:     entry.UserID,
					UserEmail:  entry.UserEmail,
					IPAddress:  entry.IPAddress,
					UserAgent:  entry.UserAgent,
					ResourceID: entry.ResourceID,
					Details:    entry.Details,
					Success:    entry.Success,
				},
			)
		}()
	}
	l.slog.InfoContext(
		ctx, "audit",
		slog.String("action", string(entry.Action)),
		slog.String("user_id", entry.UserID),
		slog.String("user_email", entry.UserEmail),
		slog.String("resource_id", entry.ResourceID),
		slog.Bool("success", entry.Success),
	)
}

// AuditC is the convenience variant for HTTP handlers.
// Extracts user_id, ip, and user_agent from the Echo context automatically.
func (l *Logger) AuditC(c *echo.Context, action AuditAction, resourceID string, err error, details string) {
	userID, _ := c.Get("user_id").(string)
	userEmail, _ := c.Get("user_email").(string)

	if err != nil {
		if details != "" {
			details += ": " + err.Error()
		} else {
			details = err.Error()
		}
	}

	l.Audit(
		c.Request().Context(), AuditEntry{
			Action:     action,
			UserID:     userID,
			UserEmail:  userEmail,
			IPAddress:  c.RealIP(),
			UserAgent:  c.Request().Header.Get("User-Agent"),
			ResourceID: resourceID,
			Details:    details,
			Success:    err == nil,
		},
	)
}

// ── GORM logger adapter ───────────────────────────────────────────────────────

// NewGORMLogger returns a gorm.Logger that emits via slog at Debug level.
func NewGORMLogger(isDev bool, slowQueryMS int64) gormlogger.Interface {
	threshold := time.Duration(slowQueryMS) * time.Millisecond
	if threshold == 0 {
		threshold = 500 * time.Millisecond
	}
	return gormlogger.New(
		&gormSlogWriter{},
		gormlogger.Config{
			SlowThreshold:             threshold,
			LogLevel:                  gormlogger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  isDev,
		},
	)
}

type gormSlogWriter struct{}

func (w *gormSlogWriter) Printf(format string, args ...interface{}) {
	slog.Debug("gorm", slog.String("msg", fmt.Sprintf(format, args...)))
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// prepend inserts key/value at the front of an args slice for slog.
func prepend(key, val string, rest []any) []any {
	out := make([]any, 0, len(rest)+2)
	out = append(out, slog.String(key, val))
	out = append(out, rest...)
	return out
}
