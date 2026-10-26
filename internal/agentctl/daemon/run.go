package daemon

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi"
	"github.com/josemiguelmelo/orkest/internal/core/httpserver"
)

// RunAgentctl owns a standalone control server, independent of the platform.
func RunAgentctl(ctx context.Context, logger *slog.Logger, address string, provider agentctl.RuntimeProvider, stateDir string, expectedRuns ...string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	controller := agentctl.NewRunControllerWithRuntimeProvider(ctx, provider, stateDir, logger)
	var supervisor sync.WaitGroup
	defer func() { cancel(); supervisor.Wait(); _ = controller.Close() }()
	if err := controller.Recover(expectedRuns...); err != nil {
		logger.Error("agent session recovery requires reconciliation", "reason", "restore_failed")
	}

	supervisor.Go(func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			status := controller.Status()
			if !status.RecoveryBlocked && status.TaskSessionID != "" && status.SessionID != "" && !status.ProcessAlive && (status.State == agentctl.StateExited || status.State == agentctl.StateFailed || status.State == agentctl.StateInterrupted) {
				if err := controller.Restore(); err != nil {
					logger.Error("runtime restoration requires reconciliation", "reason", "restore_failed")
				}
			}
		}
	})
	api, err := httpapi.NewServer(ctx, controller)
	if err != nil {
		return err
	}

	server := &http.Server{Addr: address, Handler: api.Handler(logger), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	return httpserver.Run(ctx, logger, "agentctl", server)
}
