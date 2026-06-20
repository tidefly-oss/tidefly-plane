package dashboard

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func huma401(msg string) error { return huma.NewError(http.StatusUnauthorized, msg) }
func huma500(msg string) error { return huma.NewError(http.StatusInternalServerError, msg) }
