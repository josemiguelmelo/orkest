package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// runHTTPServer owns listening and graceful shutdown. Callers configure HTTP
// policies and retain ownership of application resources and stream lifetimes.
func runHTTPServer(ctx context.Context, logger *slog.Logger, name string, server *http.Server) error {
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", name, err)
	}
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	logger.Info(name+" listening", "address", listener.Addr().String())
	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve %s: %w", name, err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		shutdownErr := server.Shutdown(shutdownCtx)
		if shutdownErr != nil {
			_ = server.Close()
		}
		serveErr := <-result
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		if err := errors.Join(shutdownErr, serveErr); err != nil {
			return fmt.Errorf("shutdown %s: %w", name, err)
		}
		return nil
	}
}
