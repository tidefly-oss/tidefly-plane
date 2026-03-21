package http

import "github.com/danielgtaylor/huma/v2"

func huma401(msg string) error { return huma.Error401Unauthorized(msg) }
func huma404(msg string) error { return huma.Error404NotFound(msg) }
func huma422(msg string) error { return huma.Error422UnprocessableEntity(msg) }
