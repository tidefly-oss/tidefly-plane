package caddy

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	applogger "github.com/tidefly-oss/tidefly-plane/internal/logger"
	"github.com/tidefly-oss/tidefly-plane/internal/services/runtime"
)

const caddyContainerName = "tidefly_caddy"

// CaddyLogEntry represents any Caddy JSON log line.
// For HTTP access logs: Method/URI/Status/Size/Duration populated.
// For system logs (tls, admin, etc.): Msg populated.
type CaddyLogEntry struct {
	Timestamp  time.Time `json:"ts"`
	Level      string    `json:"level"`
	Logger     string    `json:"logger"`
	Msg        string    `json:"msg,omitempty"`
	Method     string    `json:"method,omitempty"`
	URI        string    `json:"uri,omitempty"`
	Proto      string    `json:"proto,omitempty"`
	Host       string    `json:"host,omitempty"`
	RemoteAddr string    `json:"remote_addr,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
	Status     int       `json:"status,omitempty"`
	Size       int64     `json:"size,omitempty"`
	Duration   float64   `json:"duration,omitempty"`
	Error      string    `json:"error,omitempty"`
}

// AccessLogEntry kept as alias for backwards compat with caddy_logs.go
type AccessLogEntry = CaddyLogEntry

type caddyRawLog struct {
	Timestamp float64 `json:"ts"`
	Level     string  `json:"level"`
	Logger    string  `json:"logger"`
	Msg       string  `json:"msg"`
	Request   struct {
		Method     string `json:"method"`
		URI        string `json:"uri"`
		Proto      string `json:"proto"`
		Host       string `json:"host"`
		RemoteAddr string `json:"remote_ip"`
		Headers    struct {
			UserAgent []string `json:"User-Agent"`
		} `json:"headers"`
	} `json:"request"`
	Status   int     `json:"status"`
	Size     int64   `json:"size"`
	Duration float64 `json:"duration"`
	Error    string  `json:"err,omitempty"`
}

type LogWatcher struct {
	rt  runtime.Runtime
	log *applogger.Logger
}

func NewLogWatcher(rt runtime.Runtime, log *applogger.Logger) *LogWatcher {
	return &LogWatcher{rt: rt, log: log}
}

func (w *LogWatcher) Stream(ctx context.Context) (<-chan CaddyLogEntry, error) {
	rc, err := w.rt.ContainerLogs(
		ctx, caddyContainerName, runtime.LogOptions{
			Follow: true, Tail: "50", Timestamps: false,
		},
	)
	if err != nil {
		return nil, err
	}
	ch := make(chan CaddyLogEntry, 64)
	go func() {
		defer close(ch)
		defer func() { _ = rc.Close() }()
		w.readStream(ctx, rc, ch)
	}()
	return ch, nil
}

func (w *LogWatcher) readStream(ctx context.Context, rc io.Reader, ch chan<- CaddyLogEntry) {
	hdr := make([]byte, 8)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if _, err := io.ReadFull(rc, hdr); err != nil {
			return
		}
		size := binary.BigEndian.Uint32(hdr[4:])
		if size == 0 {
			continue
		}
		line := make([]byte, size)
		if _, err := io.ReadFull(rc, line); err != nil {
			return
		}
		entry, ok := parseLine(strings.TrimSpace(string(line)))
		if !ok {
			continue
		}
		select {
		case ch <- entry:
		case <-ctx.Done():
			return
		}
	}
}

func (w *LogWatcher) Tail(ctx context.Context, lines int) ([]CaddyLogEntry, error) {
	rc, err := w.rt.ContainerLogs(
		ctx, caddyContainerName, runtime.LogOptions{
			Follow: false, Tail: itoa(lines), Timestamps: false,
		},
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	var entries []CaddyLogEntry
	scanner := bufio.NewScanner(rc)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) > 8 {
			line = strings.TrimSpace(line[8:])
		}
		if entry, ok := parseLine(line); ok {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func parseLine(line string) (CaddyLogEntry, bool) {
	if line == "" || line[0] != '{' {
		return CaddyLogEntry{}, false
	}
	var raw caddyRawLog
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return CaddyLogEntry{}, false
	}
	// Filter healthcheck spam: Wget on admin /config/
	if raw.Logger == "admin.api" &&
		strings.Contains(raw.Request.URI, "/config/") &&
		len(raw.Request.Headers.UserAgent) > 0 &&
		strings.Contains(raw.Request.Headers.UserAgent[0], "Wget") {
		return CaddyLogEntry{}, false
	}
	ua := ""
	if len(raw.Request.Headers.UserAgent) > 0 {
		ua = raw.Request.Headers.UserAgent[0]
	}
	sec := int64(raw.Timestamp)
	nsec := int64((raw.Timestamp - float64(sec)) * 1e9)
	return CaddyLogEntry{
		Timestamp:  time.Unix(sec, nsec).UTC(),
		Level:      raw.Level,
		Logger:     raw.Logger,
		Msg:        raw.Msg,
		Method:     raw.Request.Method,
		URI:        raw.Request.URI,
		Proto:      raw.Request.Proto,
		Host:       raw.Request.Host,
		RemoteAddr: raw.Request.RemoteAddr,
		UserAgent:  ua,
		Status:     raw.Status,
		Size:       raw.Size,
		Duration:   raw.Duration,
		Error:      raw.Error,
	}, true
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}

func (c *Client) RegisterDashboard(ctx context.Context) error {
	if !c.cfg.Enabled || c.cfg.BaseDomain == "" {
		return nil
	}
	// Dashboard UI → dashboard.base_domain
	if err := c.AddHTTPRoute(ctx,
		"tidefly-plane-dashboard",
		"dashboard."+c.cfg.BaseDomain,
		"tidefly_ui:3000",
	); err != nil {
		return fmt.Errorf("dashboard route: %w", err)
	}
	// API → tidefly.base_domain
	if err := c.AddHTTPRoute(ctx,
		"tidefly-plane-api",
		"tidefly."+c.cfg.BaseDomain,
		"tidefly_backend:8181",
	); err != nil {
		return fmt.Errorf("api route: %w", err)
	}
	return nil
}
