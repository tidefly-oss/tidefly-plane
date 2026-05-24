package middleware

import (
	"net/http"
	"sync"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// ── Request ID ────────────────────────────────────────────────────────────────

// RequestID injects a unique request ID into every request.
// Uses chi's built-in implementation.
func RequestID() func(http.Handler) http.Handler {
	return chimiddleware.RequestID
}

// ── Rate Limiter ──────────────────────────────────────────────────────────────

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    int
	window  time.Duration
}

type bucket struct {
	count   int
	resetAt time.Time
}

func newRateLimiter(rate int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		window:  window,
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			rl.mu.Lock()
			now := time.Now()
			for ip, b := range rl.buckets {
				if now.After(b.resetAt) {
					delete(rl.buckets, ip)
				}
			}
			rl.mu.Unlock()
		}
	}()
	return rl
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	b, ok := rl.buckets[ip]
	if !ok || now.After(b.resetAt) {
		rl.buckets[ip] = &bucket{count: 1, resetAt: now.Add(rl.window)}
		return true
	}
	if b.count >= rl.rate {
		return false
	}
	b.count++
	return true
}

// RateLimit limits requests per IP. Uses chi's RealIP header if set.
func RateLimit(rate int, window time.Duration) func(http.Handler) http.Handler {
	rl := newRateLimiter(rate, window)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := chimiddleware.GetReqID(r.Context())
			// Use X-Real-IP / X-Forwarded-For if available (set by chi RealIP middleware)
			if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
				ip = realIP
			} else {
				ip = r.RemoteAddr
			}
			if !rl.allow(ip) {
				http.Error(w, `{"message":"too many requests — please slow down"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitAuth is a stricter limiter for auth endpoints — 10 req/min per IP.
func RateLimitAuth() func(http.Handler) http.Handler {
	return RateLimit(10, time.Minute)
}

// RateLimitAPI is the standard API limiter — 300 req/min per IP.
func RateLimitAPI() func(http.Handler) http.Handler {
	return RateLimit(300, time.Minute)
}
