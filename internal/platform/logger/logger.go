// Package logger provides structured logging, audit logging, and notification integration for Tidefly.
package logger

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

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
	"aborting with incomplete response",
}

var internalContainers = []string{
	"tidefly_caddy", "tidefly_backend", "tidefly_postgres", "tidefly_redis",
	"tidefly_caddy_dev", "tidefly_backend_dev", "tidefly_postgres_dev", "tidefly_redis_dev",
}

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

	AuditContainerDeploy  AuditAction = "container.manifest"
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

var auditActionsToNotify = map[AuditAction]bool{
	AuditLoginFailed:     true,
	AuditAdminUserDelete: true,
	AuditProjectDelete:   true,
	AuditVolumeDelete:    true,
	AuditNetworkDelete:   true,
}

type DBLogLevel int

const (
	DBLogNone DBLogLevel = iota
	DBLogErrorAndAbove
	DBLogWarnAndAbove
)

type NotificationUpsertFn func(ctx context.Context, sourceID, sourceName string, severity models.NotificationSeverity, msg string) error
type NotificationSendFn func(title, message, level string)

type Logger struct {
	slog        *slog.Logger
	db          *gorm.DB
	dbLogLevel  DBLogLevel
	notifUpsert NotificationUpsertFn
	notifSend   NotificationSendFn
}

type Options struct {
	DBLogLevel DBLogLevel
}

type Option func(*Options)

func WithDBLogLevel(l DBLogLevel) Option {
	return func(o *Options) { o.DBLogLevel = l }
}

func New(isDev bool, _ *gorm.DB, opts ...Option) (*Logger, error) {
	o := &Options{DBLogLevel: DBLogWarnAndAbove}
	for _, opt := range opts {
		opt(o)
	}
	level := slog.LevelInfo
	if isDev {
		level = slog.LevelDebug
	}
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return &Logger{
		slog:       slog.New(handler),
		dbLogLevel: o.DBLogLevel,
	}, nil
}

func (l *Logger) SetDB(db *gorm.DB) { l.db = db }

func (l *Logger) SetNotifier(upsert NotificationUpsertFn, send NotificationSendFn) {
	l.notifUpsert = upsert
	l.notifSend = send
}

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
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	if l.dbLogLevel <= DBLogErrorAndAbove {
		l.writeAppLog("ERROR", component, msg, errStr, "")
	}
	l.sendNotification("ERROR", component, msg, errStr)
}

func (l *Logger) Errorw(component, msg string, args ...any) {
	l.slog.Error(msg, prepend("component", component, args)...)
	errStr := extractArgValue(args, "error", "err")
	if l.dbLogLevel <= DBLogErrorAndAbove {
		l.writeAppLog("ERROR", component, msg, errStr, "")
	}
	l.sendNotification("ERROR", component, msg, errStr)
}

func (l *Logger) Warnw(component, msg string, args ...any) {
	l.slog.Warn(msg, prepend("component", component, args)...)
	errStr := extractArgValue(args, "error", "err")
	if l.dbLogLevel <= DBLogWarnAndAbove {
		l.writeAppLog("WARN", component, msg, errStr, "")
	}
}

func (l *Logger) Fatal(component, msg string, err error) {
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	l.writeAppLog("FATAL", component, msg, errStr, "")
	l.sendNotification("FATAL", component, msg, errStr)
	l.slog.Error(msg,
		slog.String("component", component),
		slog.String("error", errStr),
		slog.String("level", "FATAL"),
	)
	os.Exit(1)
}

func (l *Logger) WriteHTTPError(status int, method, path string) {
	l.writeAppLog("ERROR", "http", fmt.Sprintf("%s %s", method, path), fmt.Sprintf("status %d", status), "")
}

func (l *Logger) ContainerEvent(level, containerID, containerName, msg, errDetail string) {
	if isInternalContainer(containerName) {
		return
	}
	component := "container:" + containerName
	l.slog.Info("container-event",
		slog.String("level", level),
		slog.String("container_id", containerID),
		slog.String("container_name", containerName),
		slog.String("msg", msg),
	)
	l.writeAppLog(level, component, msg, errDetail, containerID)
}

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

func (l *Logger) sendNotification(level, component, msg, errStr string) {
	if l.notifUpsert == nil {
		return
	}
	for _, ignore := range ignoredMessages {
		if strings.Contains(msg, ignore) || strings.Contains(errStr, ignore) {
			return
		}
	}
	severity := models.SeverityError
	if level == "FATAL" {
		severity = models.SeverityFatal
	}
	fullMsg := msg
	if errStr != "" {
		fullMsg = msg + ": " + errStr
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = l.notifUpsert(ctx, "plane:"+component, component, severity, fullMsg)
	}()
	if l.notifSend != nil && (level == "ERROR" || level == "FATAL") {
		go func() {
			l.notifSend(
				fmt.Sprintf("[%s] %s", level, component),
				fullMsg,
				"error",
			)
		}()
	}
}

func isInternalContainer(name string) bool {
	for _, internal := range internalContainers {
		if strings.Contains(name, internal) {
			return true
		}
	}
	return false
}

// extractArgValue extracts a string value from key-value args by matching any of the given keys.
func extractArgValue(args []any, keys ...string) string {
	for i := 0; i+1 < len(args); i += 2 {
		k, ok := args[i].(string)
		if !ok {
			continue
		}
		for _, key := range keys {
			if k == key {
				return fmt.Sprintf("%v", args[i+1])
			}
		}
	}
	return ""
}

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

// auditIPKey and auditUAKey are the context keys injected by middleware.RequireAuthHuma.
// Defined here as unexported types to avoid import cycles — middleware sets them via
// the same key types exported from the middleware package.
// We read them via a shared interface: middleware.IPFromCtx / middleware.UserAgentFromCtx.
// To avoid the import cycle we accept an optional ContextEnricher.

// ContextEnricher extracts request metadata from a context.
// Implemented by the middleware package and injected at app startup.
type ContextEnricher interface {
	IP(ctx context.Context) string
	UserAgent(ctx context.Context) string
	UserEmail(ctx context.Context) string
}

var globalEnricher ContextEnricher

// SetContextEnricher wires the middleware enricher into the logger.
// Called once at app startup to avoid import cycles.
func SetContextEnricher(e ContextEnricher) {
	globalEnricher = e
}

func (l *Logger) Audit(ctx context.Context, entry AuditEntry) {
	// Auto-fill IP, UserAgent, UserEmail from context if not explicitly set
	if globalEnricher != nil {
		if entry.IPAddress == "" {
			entry.IPAddress = globalEnricher.IP(ctx)
		}
		if entry.UserAgent == "" {
			entry.UserAgent = globalEnricher.UserAgent(ctx)
		}
		if entry.UserEmail == "" {
			entry.UserEmail = globalEnricher.UserEmail(ctx)
		}
	}

	if l.db != nil {
		go func() {
			l.db.Create(&models.AuditLog{
				Action:     string(entry.Action),
				UserID:     entry.UserID,
				UserEmail:  entry.UserEmail,
				IPAddress:  entry.IPAddress,
				UserAgent:  entry.UserAgent,
				ResourceID: entry.ResourceID,
				Details:    entry.Details,
				Success:    entry.Success,
			})
		}()
	}
	l.slog.InfoContext(ctx, "audit",
		slog.String("action", string(entry.Action)),
		slog.String("user_id", entry.UserID),
		slog.String("user_email", entry.UserEmail),
		slog.String("ip", entry.IPAddress),
		slog.String("resource_id", entry.ResourceID),
		slog.Bool("success", entry.Success),
	)
	l.maybeNotifyAudit(entry)
}

func (l *Logger) maybeNotifyAudit(entry AuditEntry) {
	if l.notifUpsert == nil {
		return
	}
	alwaysNotify := auditActionsToNotify[entry.Action]
	if !alwaysNotify && entry.Success {
		return
	}
	severity := models.SeverityWarn
	if !entry.Success {
		severity = models.SeverityError
	}
	if entry.Action == AuditLoginFailed {
		severity = models.SeverityError
	}
	msg := fmt.Sprintf("audit: %s", entry.Action)
	if entry.UserEmail != "" {
		msg += " by " + entry.UserEmail
	}
	if !entry.Success {
		msg += " [FAILED]"
		if entry.Details != "" {
			msg += ": " + entry.Details
		}
	}
	sourceID := fmt.Sprintf("audit:%s:%s", entry.Action, entry.ResourceID)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = l.notifUpsert(ctx, sourceID, "audit", severity, msg)
	}()
	if l.notifSend != nil && !entry.Success {
		go func() {
			l.notifSend(
				fmt.Sprintf("[AUDIT] %s", entry.Action),
				msg,
				"warn",
			)
		}()
	}
}

func (l *Logger) AuditR(r *http.Request, action AuditAction, resourceID string, err error, details string) {
	if err != nil {
		if details != "" {
			details += ": " + err.Error()
		} else {
			details = err.Error()
		}
	}
	l.Audit(r.Context(), AuditEntry{
		Action:     action,
		IPAddress:  r.RemoteAddr,
		UserAgent:  r.Header.Get("User-Agent"),
		ResourceID: resourceID,
		Details:    details,
		Success:    err == nil,
	})
}

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

func (w *gormSlogWriter) Printf(format string, args ...any) {
	slog.Debug("gorm", slog.String("msg", fmt.Sprintf(format, args...)))
}

func prepend(key, val string, rest []any) []any {
	out := make([]any, 0, len(rest)+2)
	out = append(out, slog.String(key, val))
	out = append(out, rest...)
	return out
}
