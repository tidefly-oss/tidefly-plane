package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
)

// RateLimiter is a simple in-memory token bucket rate limiter per IP.
// For production with multiple instances, replace with a Redis-backed limiter.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    int           // max requests
	window  time.Duration // per window
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
	// Cleanup goroutine — remove expired buckets every minute
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

// RateLimit returns a middleware that limits requests per IP.
// rate = max requests, window = time window (e.g. 100 req / minute).
func RateLimit(rate int, window time.Duration) echo.MiddlewareFunc {
	rl := newRateLimiter(rate, window)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if !rl.allow(c.RealIP()) {
				return c.JSON(
					http.StatusTooManyRequests, map[string]string{
						"message": "too many requests — please slow down",
					},
				)
			}
			return next(c)
		}
	}
}

// RateLimitAuth is a stricter limiter for auth endpoints (login, password reset).
// 10 requests per minute per IP by default.
func RateLimitAuth() echo.MiddlewareFunc {
	return RateLimit(10, time.Minute)
}

// RateLimitAPI is the standard API limiter.
// 300 requests per minute per IP by default.
func RateLimitAPI() echo.MiddlewareFunc {
	return RateLimit(300, time.Minute)
}
