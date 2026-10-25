package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi"
)

// RunAgentctl owns a standalone control server, independent of the platform.
func RunAgentctl(ctx context.Context, logger *slog.Logger, address string, provider agentctl.Provider, stateDir string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	controller := agentctl.NewRunControllerWithProvider(ctx, provider, stateDir)
	defer func() { _ = controller.Close() }()
	api, err := httpapi.NewServer(ctx, controller)
	if err != nil {
		return err
	}
	server := &http.Server{Addr: address, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	return runHTTPServer(ctx, logger, "agentctl", server)
}
