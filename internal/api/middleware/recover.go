package middleware

import (
	"fmt"

	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/labstack/echo/v5"
)

func Recover(log *logger.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) (err error) {
			defer func() {
				if r := recover(); r != nil {
					log.Error("recover", "panic", fmt.Errorf("%v", r))
					err = echo.NewHTTPError(500, "internal server error")
				}
			}()
			return next(c)
		}
	}
}
