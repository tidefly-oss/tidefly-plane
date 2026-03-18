package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/tidefly-oss/tidefly-backend/internal/bootstrap"
)

func main() {
	app, cleanup, err := bootstrap.InitializeApp()
	if err != nil {
		_, err := fmt.Fprintf(os.Stderr, "init error: %v\n", err)
		if err != nil {
			return
		}
		os.Exit(1)
	}
	defer cleanup()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx); err != nil {
		_, err := fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		if err != nil {
			return
		}
		os.Exit(1)
	}
}
