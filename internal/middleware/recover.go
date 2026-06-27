package middleware

import (
	"fmt"
	"net/http"

	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
)

// Recover catches panics, logs them and returns HTTP 500.
func Recover(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("recover", "panic", fmt.Errorf("%v", rec))
					http.Error(w, `{"message":"internal server error"}`, http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
