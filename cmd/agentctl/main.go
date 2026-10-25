package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/josemiguelmelo/orkest/internal/app"
)

func main() {
	address := flag.String("listen", "127.0.0.1:8081", "agentctl HTTP listen address")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.RunAgentctl(ctx, logger, *address); err != nil {
		logger.Error("agentctl stopped", "error", err)
		os.Exit(1)
	}
}
