package orchestration

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

var ErrAgentRunClosed = errors.New("agent run completed and process closed")

type LaunchCompletionService struct {
	repo       LaunchRepository
	controller AgentProcessController
	background *backgroundWork
	mu         sync.Mutex
	pending    map[string]orchestrationmodel.Launch
	reported   map[string]bool
	slots      chan struct{}
}

func NewLaunchCompletionService(ctx context.Context, repo LaunchRepository, controller AgentProcessController) (*LaunchCompletionService, error) {
	if ctx == nil || repo == nil || controller == nil {
		return nil, errors.New("launch repository and process controller are required")
	}
	service := &LaunchCompletionService{repo: repo, controller: controller, background: newBackgroundWork(ctx), pending: make(map[string]orchestrationmodel.Launch), reported: make(map[string]bool), slots: make(chan struct{}, 4)}
	service.background.Go(service.runCleanup)
	return service, nil
}

func (s *LaunchCompletionService) AgentctlRunning(ctx context.Context, launch orchestrationmodel.Launch) (bool, error) {
	if strings.TrimSpace(launch.ProcessID) != "" {
		return s.controller.AgentctlRunning(ctx, launch.ProcessID)
	}
	return s.controller.AgentctlRunningByRunID(ctx, launch.RunID)
}

// HandleEvent durably records terminal intent before stopping the process, so a
// platform restart can finish cleanup if Docker shutdown was interrupted.
func (s *LaunchCompletionService) HandleEvent(ctx context.Context, launch orchestrationmodel.Launch, event orchestrationmodel.Event) error {
	jobOwned := false
	if owner, ok := s.repo.(interface {
		IsJobRun(context.Context, string) (bool, error)
	}); ok && (event.Payload.Type == orchestrationmodel.EventPromptCompleted || event.Payload.Type == orchestrationmodel.EventPromptFailed) {
		var err error
		jobOwned, err = owner.IsJobRun(ctx, launch.RunID)
		if err != nil {
			return err
		}
	}
	var pendingStatus orchestrationmodel.LaunchStatus
	switch event.Payload.Type {
	case orchestrationmodel.EventRunFailed:
		pendingStatus = orchestrationmodel.LaunchFailedPendingClose
	case orchestrationmodel.EventRunCompleted:
		pendingStatus = orchestrationmodel.LaunchCompletedPendingClose
	case orchestrationmodel.EventPromptCompleted:
		if event.Payload.TaskSessionID != "" && !jobOwned {
			return s.repo.SaveEvent(ctx, event)
		}
		pendingStatus = orchestrationmodel.LaunchCompletedPendingClose
	case orchestrationmodel.EventPromptFailed:
		if event.Payload.TaskSessionID != "" && !jobOwned {
			return s.repo.SaveEvent(ctx, event)
		}
		pendingStatus = orchestrationmodel.LaunchFailedPendingClose
	default:
		return s.repo.SaveEvent(ctx, event)
	}
	if err := s.repo.SaveTerminalEvent(ctx, event, pendingStatus); err != nil {
		return fmt.Errorf("persist terminal agent event: %w", err)
	}
	launch.Status = pendingStatus
	if err := s.ClosePending(ctx, launch); err != nil {
		return err
	}
	return ErrAgentRunClosed
}

// RecoverPending retries process cleanup for runs that reached a terminal event
// before the previous platform process finished stopping the container.
func (s *LaunchCompletionService) RecoverPending(ctx context.Context) error {
	launches, err := s.repo.ListReconnectableLaunches(ctx)
	if err != nil {
		return fmt.Errorf("load launches pending process cleanup: %w", err)
	}
	for _, launch := range launches {
		if launch.Status != orchestrationmodel.LaunchCompletedPendingClose && launch.Status != orchestrationmodel.LaunchFailedPendingClose {
			continue
		}
		s.ScheduleClose(launch)
	}
	return nil
}

// ScheduleClose retains terminal intent for bounded, independently retried cleanup.
func (s *LaunchCompletionService) ScheduleClose(launch orchestrationmodel.Launch) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.pending[launch.RunID]; !exists {
		s.pending[launch.RunID] = launch
	}
}

func (s *LaunchCompletionService) CloseAbandoned(_ context.Context, launch orchestrationmodel.Launch, status orchestrationmodel.LaunchStatus) {
	if status != orchestrationmodel.LaunchCompletedPendingClose && status != orchestrationmodel.LaunchFailedPendingClose {
		return
	}
	launch.Status = status
	s.ScheduleClose(launch)
}

func (s *LaunchCompletionService) runCleanup(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		s.mu.Lock()
		launches := make([]orchestrationmodel.Launch, 0, len(s.pending))
		for _, launch := range s.pending {
			launches = append(launches, launch)
		}
		s.mu.Unlock()
		reconcileBatch(ctx, launches, func(ctx context.Context, launch orchestrationmodel.Launch) {
			ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			if err := s.repo.SetLaunchStatus(ctx, PlanLaunchStatus(launch.RunID, launch.Status)); err != nil {
				s.reportCleanupFailure(launch, "terminal_intent_unavailable")
				return
			}
			if err := s.ClosePending(ctx, launch); err != nil {
				s.reportCleanupFailure(launch, "cleanup_unavailable")
				return
			}
			s.mu.Lock()
			delete(s.pending, launch.RunID)
			delete(s.reported, launch.RunID)
			s.mu.Unlock()
		})
	}
}

// Close joins terminal cleanup before its database and executor clients close.
func (s *LaunchCompletionService) Close() { s.background.Close() }

func (s *LaunchCompletionService) ClosePending(ctx context.Context, launch orchestrationmodel.Launch) error {
	finalStatus := orchestrationmodel.LaunchCompleted
	if launch.Status == orchestrationmodel.LaunchFailedPendingClose {
		finalStatus = orchestrationmodel.LaunchFailed
	} else if launch.Status != orchestrationmodel.LaunchCompletedPendingClose {
		return fmt.Errorf("launch %q is not pending process cleanup", launch.RunID)
	}
	admissionCtx, stopAdmission := context.WithTimeout(ctx, 15*time.Second)
	defer stopAdmission()
	select {
	case s.slots <- struct{}{}:
	case <-admissionCtx.Done():
		return admissionCtx.Err()
	}
	defer func() { <-s.slots }()
	closeCtx, cancelClose := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	var closeErr error
	if strings.TrimSpace(launch.ProcessID) != "" {
		closeErr = s.controller.TerminateAgentctl(closeCtx, launch.ProcessID)
	} else if strings.TrimSpace(launch.RunID) != "" {
		closeErr = s.controller.TerminateAgentctlByRunID(closeCtx, launch.RunID)
	} else {
		closeErr = errors.New("terminal launch has no process identity")
	}
	cancelClose()
	if closeErr != nil {
		return fmt.Errorf("terminate agent process: %w", closeErr)
	}
	statusCtx, cancelStatus := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancelStatus()
	if err := s.repo.SetLaunchStatus(statusCtx, PlanLaunchStatus(launch.RunID, finalStatus)); err != nil {
		return fmt.Errorf("persist completed agent launch: %w", err)
	}
	return nil
}

func (s *LaunchCompletionService) reportCleanupFailure(launch orchestrationmodel.Launch, reason string) {
	s.mu.Lock()
	report := !s.reported[launch.RunID]
	s.reported[launch.RunID] = true
	s.mu.Unlock()
	if report {
		slog.Default().Warn("terminal cleanup deferred", "run_id", launch.RunID, "workspace_id", launch.WorkspaceID, "reason", reason)
	}
}
