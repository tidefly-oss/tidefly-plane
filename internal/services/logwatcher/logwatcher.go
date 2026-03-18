package logwatcher

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tidefly-oss/tidefly-backend/internal/config"
	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
	"github.com/tidefly-oss/tidefly-backend/internal/services/notifications"
	notifiersvc "github.com/tidefly-oss/tidefly-backend/internal/services/notifier"
	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
)

// ── Patterns ──────────────────────────────────────────────────────────────────

var levelPatterns = []struct {
	level   string
	pattern *regexp.Regexp
}{
	{"FATAL", regexp.MustCompile(`(?i)\b(fatal|panic)\b`)},
	{"WARN", regexp.MustCompile(`(?i)\bwarn(ing)?\b`)},
	{"WARN", regexp.MustCompile(`(?i)\bdeprecated\b`)},
	{"ERROR", regexp.MustCompile(`(?i)\b(error|err:|exception|fail(ed|ure)?|critical)\b`)},
}

var downgradePatternsToWarn = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^error:\s+in\s+\d+\+`),
	regexp.MustCompile(`(?i)^error:\s+.*(docker image|pg_upgrade|pg_ctlcluster|mount point|subdirector)`),
}

var blockStartPatterns = []struct {
	pattern  *regexp.Regexp
	maxLines int
	summary  string
}{
	{
		pattern:  regexp.MustCompile(`(?i)^error:\s+in\s+\d+\+`),
		maxLines: 3,
		summary:  "PostgreSQL upgrade required: run pg_upgrade or use a fresh volume",
	},
}

var noisePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)error_log\s*=`),
	regexp.MustCompile(`(?i)log_error\s*=`),
	regexp.MustCompile(`(?i)\b(no error|0 error)`),
	regexp.MustCompile(`(?i)without.?error`),
	regexp.MustCompile(`(?i)error_reporting`),
	regexp.MustCompile(`(?i)loglevel\s*[=:]\s*\w*(error|warn)`),
	regexp.MustCompile(`(?i)^\s*//.*\b(error|warn)\b`),
	regexp.MustCompile(`(?i)^\s*--.*\b(error|warn)\b`),
	regexp.MustCompile(`(?i)^(see also|counter to|this is usually|the suggested|discussion around|format which|major-version|postgresql itself|see https?://)`),
	regexp.MustCompile(`(?i)^/var/lib/postgresql`),
	regexp.MustCompile(`(?i)^(allowing usage|boundary issues|upgrading the underlying)`),
}

var stripTimestampRe = regexp.MustCompile(
	`^(\d+:[A-Z]\s+\d+\s+\w+\s+\d{4}\s+[\d:.]+\s+)?` +
		`(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}[.,\d]*\s*(?:UTC|Z|[+-]\d{2}:?\d{2})?\s+)?` +
		`(\[\d+\]\s+)?`,
)

var redisHashRe = regexp.MustCompile(`^#\s+`)

func cleanLine(line string) string {
	cleaned := strings.TrimSpace(stripTimestampRe.ReplaceAllString(line, ""))
	cleaned = redisHashRe.ReplaceAllString(cleaned, "")
	return strings.TrimSpace(cleaned)
}

// ── Watcher ───────────────────────────────────────────────────────────────────

type Watcher struct {
	rt          runtime.Runtime
	log         *logger.Logger
	cfg         config.LogWatcherConfig
	notifSvc    *notifications.Service
	notifierSvc *notifiersvc.Service

	mu       sync.Mutex
	watching map[string]context.CancelFunc
	scanned  map[string]struct{}
	dedup    map[string]time.Time
}

func New(
	rt runtime.Runtime,
	log *logger.Logger,
	cfg config.LogWatcherConfig,
	notifSvc *notifications.Service,
	notifierSvc *notifiersvc.Service,
) *Watcher {
	return &Watcher{
		rt:       rt,
		log:      log,
		cfg:      cfg,
		notifSvc: notifSvc, notifierSvc: notifierSvc,
		watching: make(map[string]context.CancelFunc),
		scanned:  make(map[string]struct{}),
		dedup:    make(map[string]time.Time),
	}
}

func (w *Watcher) Run(ctx context.Context) {
	w.log.Info("logwatcher", "container log watcher started")
	w.reconcile(ctx)

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info("logwatcher", "container log watcher stopping")
			w.stopAll()
			return
		case <-ticker.C:
			w.reconcile(ctx)
		}
	}
}

func (w *Watcher) reconcile(ctx context.Context) {
	containers, err := w.rt.ListContainers(ctx, true)
	if err != nil {
		w.log.Warn("logwatcher", fmt.Sprintf("failed to list containers for log watching: %v", err))
		return
	}

	runningIDs := make(map[string]struct{})
	for _, c := range containers {
		if c.Status == runtime.StatusRunning {
			runningIDs[c.ID] = struct{}{}
		}
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	for id, cancel := range w.watching {
		if _, running := runningIDs[id]; !running {
			cancel()
			delete(w.watching, id)
		}
	}

	for _, c := range containers {
		switch c.Status {
		case runtime.StatusRunning:
			if _, watching := w.watching[c.ID]; watching {
				continue
			}
			cCtx, cancel := context.WithCancel(ctx)
			w.watching[c.ID] = cancel
			go w.watchRunning(cCtx, c)

		case runtime.StatusExited, runtime.StatusStopped:
			if _, done := w.scanned[c.ID]; done {
				continue
			}
			w.scanned[c.ID] = struct{}{}
			go w.scanExited(ctx, c)
		}
	}

	existing := make(map[string]struct{}, len(containers))
	for _, c := range containers {
		existing[c.ID] = struct{}{}
	}
	for id := range w.scanned {
		if _, ok := existing[id]; !ok {
			delete(w.scanned, id)
		}
	}
}

func (w *Watcher) stopAll() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, cancel := range w.watching {
		cancel()
	}
	w.watching = make(map[string]context.CancelFunc)
}

func (w *Watcher) watchRunning(ctx context.Context, c runtime.Container) {
	rc, err := w.rt.ContainerLogs(
		ctx, c.ID, runtime.LogOptions{
			Follow:     true,
			Tail:       w.cfg.TailLines,
			Timestamps: false,
		},
	)
	if err != nil {
		return
	}
	defer rc.Close()
	w.processStream(ctx, c, bufio.NewReaderSize(rc, 32*1024))
}

func (w *Watcher) scanExited(ctx context.Context, c runtime.Container) {
	rc, err := w.rt.ContainerLogs(
		ctx, c.ID, runtime.LogOptions{
			Follow:     false,
			Tail:       w.cfg.TailLines,
			Timestamps: false,
		},
	)
	if err != nil {
		return
	}
	defer rc.Close()
	w.processStream(ctx, c, bufio.NewReaderSize(rc, 32*1024))
}

func (w *Watcher) processStream(ctx context.Context, c runtime.Container, reader *bufio.Reader) {
	name := c.Name
	if name == "" {
		name = c.ID
	}

	type blockState struct {
		summary  string
		level    string
		lines    []string
		maxLines int
	}
	var activeBlock *blockState

	emit := func(level, msg, detail string) {
		if len(msg) > w.cfg.MaxMessageLen {
			msg = msg[:w.cfg.MaxMessageLen] + " …"
		}
		dedupKey := c.ID + ":" + level + ":" + normalizeForDedup(msg)
		if w.isDuplicate(dedupKey) {
			return
		}

		w.log.ContainerEvent(level, c.ID, name, msg, detail)

		if w.notifSvc != nil {
			severity := levelToSeverity(level)
			fullMsg := msg
			if detail != "" {
				fullMsg = msg + " — " + detail
			}
			go func() {
				upsertCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := w.notifSvc.Upsert(upsertCtx, c.ID, name, severity, fullMsg); err != nil {
					w.log.Warn("logwatcher", "failed to upsert notification: "+err.Error())
				}
			}()
		}

		if w.notifierSvc != nil && (level == "ERROR" || level == "FATAL") {
			go func() {
				checkCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				fp := notifications.Fingerprint(c.ID, strings.ToUpper(level), msg)
				if w.notifSvc != nil && !w.notifSvc.IsNew(checkCtx, fp) {
					return
				}
				w.notifierSvc.Send(
					context.Background(), notifiersvc.Event{
						Title:   fmt.Sprintf("[%s] %s", level, name),
						Message: msg,
						Level:   "error",
					},
				)
			}()
		}
	}

	flushBlock := func() {
		if activeBlock == nil {
			return
		}
		detail := strings.Join(activeBlock.lines, " | ")
		emit(activeBlock.level, activeBlock.summary, detail)
		activeBlock = nil
	}

	for {
		select {
		case <-ctx.Done():
			flushBlock()
			return
		default:
		}

		line, err := readDockerLogLine(reader)
		if err != nil {
			flushBlock()
			return
		}

		line = strings.TrimSpace(line)
		if line == "" || line == "!" {
			continue
		}

		cleaned := cleanLine(line)
		if cleaned == "" {
			continue
		}

		if activeBlock != nil {
			if !isNoise(cleaned) && len(cleaned) > 3 {
				activeBlock.lines = append(activeBlock.lines, cleaned)
			}
			if len(activeBlock.lines) >= activeBlock.maxLines {
				flushBlock()
			}
			continue
		}

		if isNoise(cleaned) {
			continue
		}

		level, matched := detectLevel(cleaned)
		if !matched {
			continue
		}

		for _, bp := range blockStartPatterns {
			if bp.pattern.MatchString(cleaned) {
				activeBlock = &blockState{
					summary:  bp.summary,
					level:    "WARN",
					maxLines: bp.maxLines,
				}
				break
			}
		}
		if activeBlock != nil {
			continue
		}

		emit(level, cleaned, "")
	}
}

func readDockerLogLine(r *bufio.Reader) (string, error) {
	header := make([]byte, 8)
	if _, err := io.ReadFull(r, header); err != nil {
		return "", err
	}
	size := binary.BigEndian.Uint32(header[4:])
	if size == 0 {
		return "", nil
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return strings.TrimRight(string(buf), "\n\r"), nil
}

func isNoise(cleaned string) bool {
	for _, n := range noisePatterns {
		if n.MatchString(cleaned) {
			return true
		}
	}
	return false
}

func detectLevel(cleaned string) (level string, matched bool) {
	for _, dp := range downgradePatternsToWarn {
		if dp.MatchString(cleaned) {
			return "WARN", true
		}
	}
	for _, lp := range levelPatterns {
		if lp.pattern.MatchString(cleaned) {
			return lp.level, true
		}
	}
	return "", false
}

func normalizeForDedup(msg string) string {
	cleaned := cleanLine(msg)
	if len(cleaned) > 80 {
		return cleaned[:80]
	}
	return cleaned
}

func (w *Watcher) isDuplicate(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	cutoff := time.Now().Add(-w.cfg.DedupWindow * 10)
	for k, t := range w.dedup {
		if t.Before(cutoff) {
			delete(w.dedup, k)
		}
	}

	if last, ok := w.dedup[key]; ok && time.Since(last) < w.cfg.DedupWindow {
		return true
	}
	w.dedup[key] = time.Now()
	return false
}

// levelToSeverity konvertiert den LogWatcher-Level-String in den Notification-Severity-Typ.
func levelToSeverity(level string) models.NotificationSeverity {
	switch strings.ToUpper(level) {
	case "FATAL":
		return models.SeverityFatal
	case "ERROR":
		return models.SeverityError
	default:
		return models.SeverityWarn
	}
}
