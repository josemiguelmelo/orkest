package executionprofile

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	executionprofilemodel "github.com/josemiguelmelo/orkest/internal/server/executionprofile/model"
)

type ExecutionProfileWorker struct {
	profileRepository ExecutionProfileRepository
	builder           Builder
	logger            *slog.Logger
	workers           int
}

func NewExecutionProfileWorker(profileRepository ExecutionProfileRepository, builder Builder, logger *slog.Logger, workers int) *ExecutionProfileWorker {
	if workers < 1 {
		workers = 1
	}
	return &ExecutionProfileWorker{profileRepository: profileRepository, builder: builder, logger: logger, workers: workers}
}

func (w *ExecutionProfileWorker) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < w.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.runWorker(ctx)
		}()
	}
	wg.Wait()
}

func (w *ExecutionProfileWorker) runWorker(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if err := w.process(ctx); err != nil {
			w.logger.Error("image build queue operation failed", "error_type", fmt.Sprintf("%T", err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *ExecutionProfileWorker) process(ctx context.Context) error {
	started := time.Now()
	owner, err := ids.NewUUID()
	if err != nil {
		return err
	}
	b, ok, err := w.profileRepository.Claim(ctx, owner)
	if err != nil || !ok {
		return err
	}
	w.logger.Info("image build claimed", "build_id", b.ID, "workspace_id", b.WorkspaceID, "state", b.Status)
	buildCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	buildID := b.ID
	done := make(chan struct{})
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-buildCtx.Done():
				return
			case <-t.C:
				if w.profileRepository.Heartbeat(buildCtx, buildID, owner) != nil {
					cancel()
					return
				}
			}
		}
	}()
	var image string
	stage := "resolve_inputs"
	if b.Status == "reconciling" {
		stage = "reconcile_registry_artifact"
		image, err = w.builder.Reconcile(buildCtx, b)
	} else {
		var resolved executionprofilemodel.Build
		resolved, err = w.builder.ResolveInputs(buildCtx, b)
		if err == nil {
			b = resolved
			stage = "persist_resolved_inputs"
		}
		if err == nil {
			err = w.profileRepository.PinInputs(buildCtx, b)
		}
		if err == nil {
			stage = "build_and_push_validate"
			image, err = w.builder.BuildAndPush(buildCtx, b)
		}
	}
	close(done)
	<-heartbeatDone
	status, diagnostic := "ready", ""
	if err != nil {
		status, diagnostic = "failed", "Image composition failed; verify input compatibility and registry access."
		if buildCtx.Err() != nil || b.Status == "reconciling" {
			status, diagnostic = "interrupted", "Build outcome is uncertain; reconcile registry state before explicit retry."
		}
		failureStage, safeMessage := SafeFailure(err)
		if failureStage == "unknown" {
			failureStage = stage
		}
		w.logger.Error("image build failed", "build_id", b.ID, "workspace_id", b.WorkspaceID,
			"stage", failureStage, "status", status, "reason", safeMessage, "error_type", fmt.Sprintf("%T", err),
			"timed_out", buildCtx.Err() != nil, "duration", time.Since(started).Round(time.Millisecond))
	} else {
		w.logger.Info("image build completed", "build_id", b.ID, "workspace_id", b.WorkspaceID,
			"status", status, "stage", stage, "duration", time.Since(started).Round(time.Millisecond))
	}
	persistCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	if err := w.profileRepository.Finish(persistCtx, b, status, image, diagnostic); err != nil {
		w.logger.Error("image build result persistence failed", "build_id", b.ID, "workspace_id", b.WorkspaceID,
			"status", status, "error_type", fmt.Sprintf("%T", err))
		return err
	}
	return nil
}
