package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/tidefly-oss/tidefly-backend/internal/bootstrap"
)

var migrateOnly = flag.Bool("migrate-only", false, "run database migrations and exit")

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	app, cleanup, err := bootstrap.InitializeApp()
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	defer cleanup()

	// If migrate-only flag is set, exit after initialization (migrations already ran)
	if *migrateOnly {
		fmt.Println("✅ Database migrations completed successfully")
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return app.Run(ctx)
}
