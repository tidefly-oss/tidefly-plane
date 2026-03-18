package auth

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/tidefly-oss/tidefly-backend/internal/config"

	"github.com/aarondl/authboss/v3"
	_ "github.com/aarondl/authboss/v3/auth"
	"github.com/aarondl/authboss/v3/defaults"
	_ "github.com/aarondl/authboss/v3/defaults"
	_ "github.com/aarondl/authboss/v3/lock"
	_ "github.com/aarondl/authboss/v3/logout"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type ABLogger struct{}

func (l *ABLogger) Info(_ string)  {}
func (l *ABLogger) Error(_ string) {}

type ErrorHandler struct{}

func (d *ErrorHandler) ServeHTTP(w http.ResponseWriter, _ error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte(`{"error":"internal server error"}`))
}

func (d *ErrorHandler) Wrap(handler func(w http.ResponseWriter, r *http.Request) error) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if err := handler(w, r); err != nil {
				d.ServeHTTP(w, err)
			}
		},
	)
}

func Setup(cfg *config.Config, db *gorm.DB, redisClient *redis.Client) (*authboss.Authboss, error) {
	storer := NewStorer(db)

	ab := authboss.New()

	ab.Config.Paths.Mount = "/auth"
	ab.Config.Paths.RootURL = "http://localhost:" + cfg.App.Port
	ab.Config.Paths.LogoutOK = "/login"

	ab.Config.Storage.Server = storer
	ab.Config.Storage.SessionState = NewSessionStorer(redisClient, cfg.Auth.SessionSecret)
	ab.Config.Storage.CookieState = NewCookieStorer()

	ab.Config.Core.Logger = &ABLogger{}

	defaults.SetCore(&ab.Config, false, false)

	ab.Config.Core.ViewRenderer = &JSONRenderer{}
	ab.Config.Core.MailRenderer = &JSONRenderer{}
	ab.Config.Core.Redirector = &JSONRedirector{}
	ab.Config.Core.ErrorHandler = &ErrorHandler{}

	ab.Config.Core.BodyReader = defaults.HTTPBodyReader{
		ReadJSON: true,
		Whitelist: map[string][]string{
			"login":  {"email", "password"},
			"logout": {},
		},
	}

	ab.Config.Modules.LockAfter = 5
	ab.Config.Modules.LockWindow = 10
	ab.Config.Modules.LockDuration = 1
	ab.Config.Modules.LogoutMethod = "POST"

	if err := ab.Init(); err != nil {
		return nil, err
	}

	return ab, nil
}

// ── JSONRenderer ──────────────────────────────────────────────────────────────

type JSONRenderer struct{}

func (r *JSONRenderer) Load(_ ...string) error { return nil }

func (r *JSONRenderer) Render(_ context.Context, _ string, data authboss.HTMLData) ([]byte, string, error) {
	if len(data) == 0 {
		return []byte("{}"), "application/json", nil
	}

	clean := make(map[string]interface{})
	for k, v := range data {
		switch k {
		case "csrfToken", "csrfField":
		default:
			clean[k] = v
		}
	}

	b, err := json.Marshal(clean)
	if err != nil {
		return nil, "", err
	}
	return b, "application/json", nil
}

// ── JSONRedirector ────────────────────────────────────────────────────────────

type JSONRedirector struct{}

func (rd *JSONRedirector) Redirect(w http.ResponseWriter, _ *http.Request, ro authboss.RedirectOptions) error {
	resp := map[string]interface{}{
		"redirectTo": ro.RedirectPath,
	}

	code := http.StatusOK

	if ro.Failure != "" {
		resp["error"] = ro.Failure
		code = http.StatusUnauthorized
	} else if ro.Success != "" {
		resp["message"] = ro.Success
	}

	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Redirect-To", ro.RedirectPath)
	w.WriteHeader(code)
	_, err = w.Write(b)
	return err
}
