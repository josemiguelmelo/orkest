package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
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
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	logger.Info("platform listening", "address", listener.Addr().String())
	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("shutdown HTTP: %w", err)
		}
		return nil
	}
}
