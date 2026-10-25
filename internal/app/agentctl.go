package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi"
)

// RunAgentctl owns a standalone control server, independent of the platform.
func RunAgentctl(ctx context.Context, logger *slog.Logger, address string, provider agentctl.Provider) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	service := agentctl.NewServiceWithProvider(ctx, provider)
	defer func() { _ = service.Close() }()
	api, err := httpapi.NewServer(ctx, service)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen agentctl: %w", err)
	}
	server := &http.Server{Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	logger.Info("agentctl listening", "address", listener.Addr().String())
	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve agentctl: %w", err)
	case <-ctx.Done():
		shutdownCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("shutdown agentctl: %w", err)
		}
		return nil
	}
}
