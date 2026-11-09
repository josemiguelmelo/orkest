package main

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/josemiguelmelo/orkest/internal/adapters/http/agentctl"
	"github.com/josemiguelmelo/orkest/internal/agentctl/repositoryobserver"
	agentrun "github.com/josemiguelmelo/orkest/internal/agentctl/run"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	"github.com/josemiguelmelo/orkest/internal/platform/httpserver"
)

// RunAgentctl owns a standalone control server, independent of the platform.
func RunAgentctl(ctx context.Context, logger *slog.Logger, address string, host agentruntime.RuntimeHost, stateDir string, expectedRunID string, scanner repositoryobserver.Scanner) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	controller := agentrun.NewRunController(ctx, host, stateDir, logger, scanner)
	var supervisor sync.WaitGroup
	defer func() { cancel(); supervisor.Wait(); _ = controller.Close() }()
	if err := controller.RecoverFromCheckpoint(expectedRunID); err != nil {
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
			if controller.CheckpointRecoveryPending() {
				_ = controller.RecoverFromCheckpoint(expectedRunID)
				continue
			}
			if controller.CanAutoRestore() {
				if err := controller.RestoreSession(); err != nil {
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
