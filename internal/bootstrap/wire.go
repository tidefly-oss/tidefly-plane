//go:build wireinject
// +build wireinject

package bootstrap

import (
	"github.com/google/wire"
)

// InitializeApp is the Wire entry point.
// Wire reads this file and generates wire_gen.go with the full wiring.
// Run: wire ./internal/bootstrap/
func InitializeApp() (*App, func(), error) {
	wire.Build(ProviderSet)
	return nil, nil, nil
}
