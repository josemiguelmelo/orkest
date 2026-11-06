// Package httpserver provides lifecycle helpers shared by the platform and agentctl.
package httpserver

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
func Run(ctx context.Context, logger *slog.Logger, name string, server *http.Server) error {
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", name, err)
	}

	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	logger.Info(name+" listening", "address", listener.Addr().String())

	select {
	case err := <-result:
		return handleError(err, name)
	case <-ctx.Done():
		return handleDone(server, result, name)
	}
}

func handleError(err error, name string) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("serve %s: %w", name, err)
}

func handleDone(server *http.Server, result chan error, name string) error {
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
