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

	applogger "github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
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
	if !settings.NotificationsEnabled {
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

	if settings.SMTPTLSEnabled {
		tlsCfg := &tls.Config{ServerName: settings.SMTPHost}
		dialer := &tls.Dialer{Config: tlsCfg}
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		client, err := smtp.NewClient(conn, settings.SMTPHost)
		if err != nil {
			return
		}
		defer func() { _ = client.Close() }()
		if auth != nil {
			if err := client.Auth(auth); err != nil {
				return
			}
		}
		if err := client.Mail(settings.SMTPFrom); err != nil {
			return
		}
		if err := client.Rcpt(settings.SMTPFrom); err != nil {
			return
		}
		w, err := client.Data()
		if err != nil {
			return
		}
		if _, err := fmt.Fprint(w, body); err != nil {
			return
		}
		if err := w.Close(); err != nil {
			return
		}
		_ = client.Quit()
	} else {
		if err := smtp.SendMail(addr, auth, settings.SMTPFrom, []string{settings.SMTPFrom}, []byte(body)); err != nil {
			n.log.Error("notifier", "send email failed", err)
		}
	}
}

func (n *Notifier) postJSON(ctx context.Context, url string, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
}

func (n *Notifier) missingURL(channel string) error {
	err := fmt.Errorf("%s webhook URL not configured", channel)
	n.log.Error("notifier", "test failed", err, "channel", channel)
	return err
}
