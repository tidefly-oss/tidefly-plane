package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aarondl/authboss/v3"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	sessionCookieName = "tidefly_session"
	sessionTTL        = 24 * time.Hour
	cookieName        = "tidefly_remember"
)

type SessionStorer struct {
	redis  *redis.Client
	secret string
}

func NewSessionStorer(redisClient *redis.Client, secret string) *SessionStorer {
	return &SessionStorer{redis: redisClient, secret: secret}
}

func (s *SessionStorer) ReadState(r *http.Request) (authboss.ClientState, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return &SessionState{id: "", values: map[string]string{}}, nil
	}

	sessionID := cookie.Value
	data, err := s.redis.Get(context.Background(), sessionKey(sessionID)).Result()
	if err != nil {
		return &SessionState{id: sessionID, values: map[string]string{}}, nil
	}

	values := map[string]string{}
	if err := json.Unmarshal([]byte(data), &values); err != nil {
		return &SessionState{id: sessionID, values: map[string]string{}}, nil
	}

	return &SessionState{id: sessionID, values: values}, nil
}

func (s *SessionStorer) WriteState(
	w http.ResponseWriter, state authboss.ClientState, events []authboss.ClientStateEvent,
) error {
	ss := state.(*SessionState)

	for _, ev := range events {
		switch ev.Kind {
		case authboss.ClientStateEventPut:
			ss.values[ev.Key] = ev.Value
		case authboss.ClientStateEventDel:
			delete(ss.values, ev.Key)
		}
	}

	if len(ss.values) == 0 {
		if ss.id != "" {
			_ = s.redis.Del(context.Background(), sessionKey(ss.id)).Err()
		}
		http.SetCookie(
			w, &http.Cookie{
				Name:     sessionCookieName,
				Value:    "",
				MaxAge:   -1,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			},
		)
		return nil
	}

	if ss.id == "" {
		ss.id = newID()
	}

	data, err := json.Marshal(ss.values)
	if err != nil {
		return fmt.Errorf("session marshal: %w", err)
	}

	if err := s.redis.Set(context.Background(), sessionKey(ss.id), data, sessionTTL).Err(); err != nil {
		return fmt.Errorf("session save: %w", err)
	}

	http.SetCookie(
		w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    ss.id,
			MaxAge:   int(sessionTTL.Seconds()),
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		},
	)

	return nil
}

type SessionState struct {
	id     string
	values map[string]string
}

func (ss *SessionState) Get(key string) (string, bool) {
	v, ok := ss.values[key]
	return v, ok
}

func sessionKey(id string) string {
	return "session:" + id
}

func newID() string {
	return uuid.New().String()
}

// ── Cookie Storer ─────────────────────────────────────────────────────────────

type CookieStorer struct{}

func NewCookieStorer() *CookieStorer {
	return &CookieStorer{}
}

func (c *CookieStorer) ReadState(r *http.Request) (authboss.ClientState, error) {
	return &CookieState{r: r}, nil
}

func (c *CookieStorer) WriteState(
	w http.ResponseWriter, state authboss.ClientState, events []authboss.ClientStateEvent,
) error {
	cs := state.(*CookieState)

	for _, ev := range events {
		switch ev.Kind {
		case authboss.ClientStateEventPut:
			http.SetCookie(
				w, &http.Cookie{
					Name:     ev.Key,
					Value:    ev.Value,
					MaxAge:   int((30 * 24 * time.Hour).Seconds()),
					Path:     "/",
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				},
			)
		case authboss.ClientStateEventDel:
			http.SetCookie(
				w, &http.Cookie{
					Name:   ev.Key,
					Value:  "",
					MaxAge: -1,
					Path:   "/",
				},
			)
		}
	}
	_ = cs
	return nil
}

type CookieState struct {
	r *http.Request
}

func (cs *CookieState) Get(key string) (string, bool) {
	cookie, err := cs.r.Cookie(key)
	if err != nil {
		return "", false
	}
	return cookie.Value, true
}
