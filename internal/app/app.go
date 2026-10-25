package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/josemiguelmelo/orkest/internal/httpapi"
)

// Run owns the platform HTTP lifecycle; it does not own agent processes.
func Run(ctx context.Context, logger *slog.Logger) error {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	server := &http.Server{Addr: addr, Handler: httpapi.NewRouter(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	return runHTTPServer(ctx, logger, "platform", server)
}
