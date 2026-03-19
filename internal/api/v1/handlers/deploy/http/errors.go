package http

import "github.com/danielgtaylor/huma/v2"

func huma400(msg string) error { return huma.Error400BadRequest(msg) }
func huma404(msg string) error { return huma.Error404NotFound(msg) }
