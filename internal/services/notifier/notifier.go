package notifier

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

	applogger "github.com/tidefly-oss/tidefly-backend/internal/logger"
	"gorm.io/gorm"

	"github.com/tidefly-oss/tidefly-backend/internal/models"
)

type Event struct {
	Title   string
	Message string
	Level   string // "info" | "warning" | "error"
}

type Service struct {
	db  *gorm.DB
	log *applogger.Logger
}

func New(db *gorm.DB, log *applogger.Logger) *Service {
	return &Service{db: db, log: log}
}

func (s *Service) Send(ctx context.Context, event Event) {
	var settings models.SystemSettings
	if err := s.db.WithContext(ctx).First(&settings).Error; err != nil {
		return
	}
	if !settings.NotificationsEnabled {
		return
	}

	if settings.SlackWebhookURL != "" {
		go s.sendSlack(settings.SlackWebhookURL, event)
	}
	if settings.DiscordWebhookURL != "" {
		go s.sendDiscord(settings.DiscordWebhookURL, event)
	}
	if settings.SMTPHost != "" && settings.SMTPFrom != "" {
		go s.sendEmail(settings, event)
	}
}

// ── Test ──────────────────────────────────────────────────────────────────────

func (s *Service) Test(ctx context.Context, channel string) error {
	var settings models.SystemSettings
	if err := s.db.WithContext(ctx).First(&settings).Error; err != nil {
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
			return s.missingURL("slack")
		}
		s.sendSlack(settings.SlackWebhookURL, event)
	case "discord":
		if settings.DiscordWebhookURL == "" {
			return s.missingURL("discord")
		}
		s.sendDiscord(settings.DiscordWebhookURL, event)
	case "email":
		if settings.SMTPHost == "" {
			return s.missingURL("smtp")
		}
		s.sendEmail(settings, event)
	default:
		return s.missingURL("unknown channel")
	}
	return nil
}

// ── Slack ─────────────────────────────────────────────────────────────────────

func (s *Service) sendSlack(webhookURL string, event Event) {
	emoji := ":white_check_mark:"
	if event.Level == "warning" {
		emoji = ":warning:"
	} else if event.Level == "error" {
		emoji = ":x:"
	}

	payload := map[string]any{
		"text": fmt.Sprintf("%s *%s*\n%s", emoji, event.Title, event.Message),
	}
	s.postJSON(webhookURL, payload)
}

// ── Discord ───────────────────────────────────────────────────────────────────

func (s *Service) sendDiscord(webhookURL string, event Event) {
	color := 0x57F287 // green
	if event.Level == "warning" {
		color = 0xFEE75C // yellow
	} else if event.Level == "error" {
		color = 0xED4245 // red
	}

	payload := map[string]any{
		"embeds": []map[string]any{
			{
				"title":       event.Title,
				"description": event.Message,
				"color":       color,
				"timestamp":   time.Now().UTC().Format(time.RFC3339),
				"footer": map[string]string{
					"text": "Tidefly",
				},
			},
		},
	}
	s.postJSON(webhookURL, payload)
}

// ── SMTP ──────────────────────────────────────────────────────────────────────

func (s *Service) sendEmail(settings models.SystemSettings, event Event) {
	s.log.Info(
		"notifier", fmt.Sprintf(
			"sending email: host=%s port=%d from=%s tls=%v",
			settings.SMTPHost, settings.SMTPPort, settings.SMTPFrom, settings.SMTPTLSEnabled,
		),
	)
	addr := fmt.Sprintf("%s:%d", settings.SMTPHost, settings.SMTPPort)

	body := strings.Join(
		[]string{
			"From: " + settings.SMTPFrom,
			"To: " + settings.SMTPFrom,
			"Subject: [Tidefly] " + event.Title,
			"MIME-Version: 1.0",
			"Content-Type: text/plain; charset=UTF-8",
			"",
			event.Message,
		}, "\r\n",
	)

	var auth smtp.Auth
	if settings.SMTPUsername != "" {
		auth = smtp.PlainAuth("", settings.SMTPUsername, settings.SMTPPassword, settings.SMTPHost)
	}

	if settings.SMTPTLSEnabled {
		tlsCfg := &tls.Config{ServerName: settings.SMTPHost}
		conn, err := tls.Dial("tcp", addr, tlsCfg)
		if err != nil {
			return
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, settings.SMTPHost)
		if err != nil {
			return
		}
		defer client.Close()
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
		fmt.Fprint(w, body)
		w.Close()
		client.Quit()
	} else {
		smtp.SendMail(addr, auth, settings.SMTPFrom, []string{settings.SMTPFrom}, []byte(body))
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (s *Service) postJSON(url string, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

func (s *Service) missingURL(channel string) error {
	err := fmt.Errorf("%s webhook URL not configured", channel)
	s.log.Error("notifier", "test failed", err, "channel", channel)
	return err
}
