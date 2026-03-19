package http

import "github.com/danielgtaylor/huma/v2"

func huma401(msg string) error { return huma.Error401Unauthorized(msg) }
