//go:build wireinject
// +build wireinject

package bootstrap

import (
	"github.com/google/wire"

	"github.com/tidefly-oss/tidefly-backend/internal/api"
)

// InitializeApp is the Wire entry point.
// Wire reads this file and generates wire_gen.go with the full wiring.
// Run: wire ./internal/bootstrap/
func InitializeApp() (*api.App, func(), error) {
	wire.Build(ProviderSet)
	return nil, nil, nil
}
