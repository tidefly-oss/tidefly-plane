package middleware

import (
	"github.com/labstack/echo/v5"
	echomiddleware "github.com/labstack/echo/v5/middleware"
)

func RequestID() echo.MiddlewareFunc {
	return echomiddleware.RequestID()
}
