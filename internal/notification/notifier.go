package notification

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"

	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/_logger"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

type Event struct {
	Title   string
	Message string
	Level   string // "info" | "warning" | "error"
}

type Notifier struct {
	db  *gorm.DB
	log *applogger.Logger
}

func NewNotifier(db *gorm.DB, log *applogger.Logger) *Notifier {
	return &Notifier{db: db, log: log}
}

func (n *Notifier) Send(ctx context.Context, event Event) {
	var settings models.SystemSettings
	if err := n.db.WithContext(ctx).First(&settings).Error; err != nil {
		return
	}
	if !settings.ExternalNotificationsEnabled {
		return
	}
	if settings.SlackWebhookURL != "" {
		go n.sendSlack(context.Background(), settings.SlackWebhookURL, event)
	}
	if settings.DiscordWebhookURL != "" {
		go n.sendDiscord(context.Background(), settings.DiscordWebhookURL, event)
	}
	if settings.SMTPHost != "" && settings.SMTPFrom != "" {
		go n.sendEmail(context.Background(), settings, event)
	}
}

func (n *Notifier) Test(ctx context.Context, channel string) error {
	var settings models.SystemSettings
	if err := n.db.WithContext(ctx).First(&settings).Error; err != nil {
		return fmt.Errorf("no settings found")
	}
	event := Event{
		Title:   "Tidefly Test Notification",
		Message: "Your notification channel is configured correctly.",
		Level:   "info",
	}
	switch channel {
	case "slack":
		if settings.SlackWebhookURL == "" {
			return n.missingURL("slack")
		}
		n.sendSlack(ctx, settings.SlackWebhookURL, event)
	case "discord":
		if settings.DiscordWebhookURL == "" {
			return n.missingURL("discord")
		}
		n.sendDiscord(ctx, settings.DiscordWebhookURL, event)
	case "email":
		if settings.SMTPHost == "" {
			return n.missingURL("smtp")
		}
		n.sendEmail(ctx, settings, event)
	default:
		return n.missingURL("unknown channel")
	}
	return nil
}

func (n *Notifier) sendSlack(ctx context.Context, webhookURL string, event Event) {
	emoji := ":white_check_mark:"
	switch event.Level {
	case "warning":
		emoji = ":warning:"
	case "error":
		emoji = ":x:"
	}
	n.postJSON(ctx, webhookURL, map[string]any{
		"text": fmt.Sprintf("%s *%s*\n%s", emoji, event.Title, event.Message),
	})
}

func (n *Notifier) sendDiscord(ctx context.Context, webhookURL string, event Event) {
	color := 0x57F287
	switch event.Level {
	case "warning":
		color = 0xFEE75C
	case "error":
		color = 0xED4245
	}
	n.postJSON(ctx, webhookURL, map[string]any{
		"embeds": []map[string]any{
			{
				"title":       event.Title,
				"description": event.Message,
				"color":       color,
				"timestamp":   time.Now().UTC().Format(time.RFC3339),
				"footer":      map[string]string{"text": "Tidefly"},
			},
		},
	})
}

func (n *Notifier) sendEmail(ctx context.Context, settings models.SystemSettings, event Event) {
	n.log.Info("notifier", fmt.Sprintf(
		"sending email: host=%s port=%d from=%s tls=%v",
		settings.SMTPHost, settings.SMTPPort, settings.SMTPFrom, settings.SMTPTLSEnabled,
	))

	addr := fmt.Sprintf("%s:%d", settings.SMTPHost, settings.SMTPPort)
	body := strings.Join([]string{
		"From: " + settings.SMTPFrom,
		"To: " + settings.SMTPFrom,
		"Subject: [Tidefly] " + event.Title,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		event.Message,
	}, "\r\n")

	var auth smtp.Auth
	if settings.SMTPUsername != "" {
		auth = smtp.PlainAuth("", settings.SMTPUsername, settings.SMTPPassword, settings.SMTPHost)
	}

	// Port 465 = direct TLS (SMTPS), port 587 = STARTTLS
	if settings.SMTPTLSEnabled && settings.SMTPPort == 465 {
		n.sendEmailDirectTLS(ctx, addr, auth, settings.SMTPHost, settings.SMTPFrom, body)
	} else {
		n.sendEmailSTARTTLS(ctx, addr, auth, settings.SMTPHost, settings.SMTPFrom, body, settings.SMTPTLSEnabled)
	}
}

// sendEmailDirectTLS connects with direct TLS (port 465 / SMTPS).
func (n *Notifier) sendEmailDirectTLS(ctx context.Context, addr string, auth smtp.Auth, host, from, body string) {
	tlsCfg := &tls.Config{ServerName: host}
	dialer := &tls.Dialer{Config: tlsCfg}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		n.log.Error("notifier", "smtp direct TLS dial failed", err)
		return
	}
	defer func() { _ = conn.Close() }()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		n.log.Error("notifier", "smtp client creation failed", err)
		return
	}
	defer func() { _ = client.Close() }()

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			n.log.Error("notifier", "smtp auth failed", err)
			return
		}
	}
	if err := client.Mail(from); err != nil {
		n.log.Error("notifier", "smtp MAIL FROM failed", err)
		return
	}
	if err := client.Rcpt(from); err != nil {
		n.log.Error("notifier", "smtp RCPT TO failed", err)
		return
	}
	w, err := client.Data()
	if err != nil {
		n.log.Error("notifier", "smtp DATA failed", err)
		return
	}
	if _, err := fmt.Fprint(w, body); err != nil {
		n.log.Error("notifier", "smtp write body failed", err)
		return
	}
	if err := w.Close(); err != nil {
		n.log.Error("notifier", "smtp close writer failed", err)
		return
	}
	if err := client.Quit(); err != nil {
		n.log.Warnw("notifier", "smtp QUIT failed", "error", err)
	}
}

// sendEmailSTARTTLS connects plain then upgrades with STARTTLS (port 587).
// If tlsEnabled is false, sends plain without TLS.
func (n *Notifier) sendEmailSTARTTLS(ctx context.Context, addr string, auth smtp.Auth, host, from, body string, tlsEnabled bool) {
	_ = ctx // smtp.Dial does not support context — use timeout via deadline if needed
	client, err := smtp.Dial(addr)
	if err != nil {
		n.log.Error("notifier", "smtp dial failed", err)
		return
	}
	defer func() { _ = client.Close() }()

	if tlsEnabled {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: host}); err != nil {
				n.log.Error("notifier", "smtp STARTTLS failed", err)
				return
			}
		} else {
			n.log.Warnw("notifier", "smtp server does not support STARTTLS", "host", host)
		}
	}

	if auth != nil {
		if err := client.Auth(auth); err != nil {
			n.log.Error("notifier", "smtp auth failed", err)
			return
		}
	}
	if err := client.Mail(from); err != nil {
		n.log.Error("notifier", "smtp MAIL FROM failed", err)
		return
	}
	if err := client.Rcpt(from); err != nil {
		n.log.Error("notifier", "smtp RCPT TO failed", err)
		return
	}
	w, err := client.Data()
	if err != nil {
		n.log.Error("notifier", "smtp DATA failed", err)
		return
	}
	if _, err := fmt.Fprint(w, body); err != nil {
		n.log.Error("notifier", "smtp write body failed", err)
		return
	}
	if err := w.Close(); err != nil {
		n.log.Error("notifier", "smtp close writer failed", err)
		return
	}
	if err := client.Quit(); err != nil {
		n.log.Warnw("notifier", "smtp QUIT failed", "error", err)
	}
}

func (n *Notifier) postJSON(ctx context.Context, url string, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		n.log.Error("notifier", "marshal payload failed", err)
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		n.log.Error("notifier", "create request failed", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		n.log.Error("notifier", "post JSON failed", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		n.log.Warnw("notifier", "webhook returned error status", "status", resp.StatusCode, "url", url)
	}
}

func (n *Notifier) missingURL(channel string) error {
	err := fmt.Errorf("%s webhook URL not configured", channel)
	n.log.Error("notifier", "test failed", err, "channel", channel)
	return err
}
