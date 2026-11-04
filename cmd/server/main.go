package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/josemiguelmelo/orkest/internal/platform/environment"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := environment.LoadEnvironment(".env"); err != nil {
		logger.Error("platform stopped", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runServer(ctx, logger); err != nil {
		logger.Error("platform stopped", "error", err)
		os.Exit(1)
	}
}
