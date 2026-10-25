package orchestration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

var ErrAgentRunClosed = errors.New("agent run completed and process closed")

type LaunchCompletionService struct {
	repo       LaunchRepository
	controller AgentProcessController
}

func NewLaunchCompletionService(repo LaunchRepository, controller AgentProcessController) (*LaunchCompletionService, error) {
	if repo == nil || controller == nil {
		return nil, errors.New("launch repository and process controller are required")
	}
	return &LaunchCompletionService{repo: repo, controller: controller}, nil
}

func (s *LaunchCompletionService) AgentctlRunning(ctx context.Context, launch Launch) (bool, error) {
	if strings.TrimSpace(launch.ProcessID) != "" {
		return s.controller.AgentctlRunning(ctx, launch.ProcessID)
	}
	return s.controller.AgentctlRunningByRunID(ctx, launch.RunID)
}

// HandleEvent durably records terminal intent before stopping the process, so a
// platform restart can finish cleanup if Docker shutdown was interrupted.
func (s *LaunchCompletionService) HandleEvent(ctx context.Context, launch Launch, event Event) error {
	var pendingStatus LaunchStatus
	switch event.Payload.Type {
	case agentctl.EventPromptCompleted:
		pendingStatus = LaunchCompletedPendingClose
	case agentctl.EventPromptFailed:
		pendingStatus = LaunchFailedPendingClose
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
		if launch.Status != LaunchCompletedPendingClose && launch.Status != LaunchFailedPendingClose {
			continue
		}
		go s.RetryClose(ctx, launch)
	}
	return nil
}

func (s *LaunchCompletionService) CloseAbandoned(ctx context.Context, launch Launch, pendingStatus LaunchStatus) {
	if pendingStatus != LaunchCompletedPendingClose && pendingStatus != LaunchFailedPendingClose {
		return
	}
	launch.Status = pendingStatus
	for {
		if err := s.repo.SetLaunchStatus(ctx, launch.RunID, pendingStatus); err == nil {
			s.RetryClose(ctx, launch)
			return
		}
		if !waitForRetry(ctx) {
			return
		}
	}
}

func (s *LaunchCompletionService) RetryClose(ctx context.Context, launch Launch) {
	for {
		if err := s.ClosePending(ctx, launch); err == nil {
			return
		}
		if !waitForRetry(ctx) {
			return
		}
	}
}

func (s *LaunchCompletionService) ClosePending(ctx context.Context, launch Launch) error {
	finalStatus := LaunchCompleted
	if launch.Status == LaunchFailedPendingClose {
		finalStatus = LaunchFailed
	} else if launch.Status != LaunchCompletedPendingClose {
		return fmt.Errorf("launch %q is not pending process cleanup", launch.RunID)
	}
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
	if err := s.repo.SetLaunchStatus(statusCtx, launch.RunID, finalStatus); err != nil {
		return fmt.Errorf("persist completed agent launch: %w", err)
	}
	return nil
}
