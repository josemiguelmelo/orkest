package orchestration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentrun "github.com/josemiguelmelo/orkest/internal/agentctl/run"
)

type AgentCommunication interface {
	Start(Launch) error
}

type connectionRun struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type ConnectionService struct {
	ctx        context.Context
	client     AgentConnectionClient
	repo       ConnectionRepository
	completion *LaunchCompletionService
	mu         sync.Mutex
	runs       map[string]connectionRun
}

func NewConnectionService(ctx context.Context, client AgentConnectionClient, repo ConnectionRepository, completion ...*LaunchCompletionService) (*ConnectionService, error) {
	if ctx == nil || client == nil || repo == nil {
		return nil, errors.New("agentctl client and launch repository are required")
	}
	service := &ConnectionService{ctx: ctx, client: client, repo: repo, runs: make(map[string]connectionRun)}
	if len(completion) > 0 {
		service.completion = completion[0]
	}
	return service, nil
}

// Start performs an initial WebSocket handshake, then reconnects in the background.
func (s *ConnectionService) Start(launch Launch) error {
	if err := s.start(launch, true, false); err != nil {
		return err
	}
	if s.completion != nil {
		if runCtx := s.runContext(launch.RunID); runCtx != nil {
			go s.monitorLaunch(runCtx, launch)
		}
	}
	return nil
}

// ReconnectRecorded resumes event streams for persisted active launches. It never
// starts, configures, or replaces an agent process.
func (s *ConnectionService) ReconnectRecorded() error {
	launches, err := s.repo.ListReconnectableLaunches(s.ctx)
	if err != nil {
		return fmt.Errorf("load launches for reconnect: %w", err)
	}
	for _, launch := range launches {
		if launch.Status == LaunchCompletedPendingClose || launch.Status == LaunchFailedPendingClose {
			continue
		}
		if s.completion != nil && (launch.Status == LaunchStarting || launch.Status == LaunchFailed) {
			pendingStatus := LaunchFailedPendingClose
			if launch.LastEventType == string(interaction.EventRunCompleted) || (launch.TaskSessionID == "" && launch.LastEventType == string(interaction.EventPromptCompleted)) {
				pendingStatus = LaunchCompletedPendingClose
			}
			go s.completion.CloseAbandoned(s.ctx, launch, pendingStatus)
			continue
		}
		if strings.TrimSpace(launch.RunID) == "" || strings.TrimSpace(launch.ProcessID) == "" || !validEndpoint(launch.Endpoint) {
			if s.completion != nil {
				go s.completion.CloseAbandoned(s.ctx, launch, LaunchFailedPendingClose)
			}
			continue
		}
		if s.completion == nil {
			if err = s.start(launch, false, true); err != nil {
				return fmt.Errorf("reconnect launch %q: %w", launch.RunID, err)
			}
			continue
		}
		go s.reconcileAndReconnect(launch)
	}
	return nil
}

func (s *ConnectionService) reconcileAndReconnect(launch Launch) {
	if launch.LastEventType == string(interaction.EventRunCompleted) || (launch.TaskSessionID == "" && launch.LastEventType == string(interaction.EventPromptCompleted)) {
		s.completion.CloseAbandoned(s.ctx, launch, LaunchCompletedPendingClose)
		return
	}
	if launch.LastEventType == string(interaction.EventRunFailed) || (launch.TaskSessionID == "" && launch.LastEventType == string(interaction.EventPromptFailed)) {
		s.completion.CloseAbandoned(s.ctx, launch, LaunchFailedPendingClose)
		return
	}
	statusCtx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	status, err := s.client.Status(statusCtx, launch.Endpoint)
	cancel()
	if err == nil && launch.TaskSessionID != "" && !matchesExecution(launch, status) {
		return
	}
	if err == nil && agentctlProcessHasEnded(status) {
		s.completion.CloseAbandoned(s.ctx, launch, LaunchFailedPendingClose)
		return
	}
	if err != nil && s.containerConfirmedStopped(s.ctx, launch) {
		s.completion.CloseAbandoned(s.ctx, launch, LaunchFailedPendingClose)
		return
	}
	s.reconcilePrompts(s.ctx, launch)
	if s.start(launch, false, true) == nil {
		if runCtx := s.runContext(launch.RunID); runCtx != nil {
			go s.monitorLaunch(runCtx, launch)
		}
	}
}

func agentctlProcessHasEnded(status agentrun.RunStatus) bool {
	if !status.Configured {
		return true
	}
	if status.TaskSessionID != "" {
		return false
	}
	if status.State == agentrun.StateFailed || status.State == agentrun.StateInterrupted || status.State == agentrun.StateExited {
		return true
	}
	if status.State == agentrun.StateStarting {
		return false
	}
	return status.Configured && (!status.ProcessAlive || !status.ExecutionAvailable)
}

func (s *ConnectionService) monitorLaunch(ctx context.Context, launch Launch) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		statusCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		status, err := s.client.Status(statusCtx, launch.Endpoint)
		cancel()
		if err != nil {
			if s.containerConfirmedStopped(ctx, launch) {
				s.cancelRun(launch.RunID)
				s.completion.CloseAbandoned(s.ctx, launch, LaunchFailedPendingClose)
				return
			}
			continue
		}
		if launch.TaskSessionID != "" && !matchesExecution(launch, status) {
			continue
		}
		if !agentctlProcessHasEnded(status) {
			s.reconcilePrompts(ctx, launch)
			continue
		}
		s.cancelRun(launch.RunID)
		s.completion.CloseAbandoned(s.ctx, launch, LaunchFailedPendingClose)
		return
	}
}

func (s *ConnectionService) containerConfirmedStopped(ctx context.Context, launch Launch) bool {
	inspectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	running, err := s.completion.AgentctlRunning(inspectCtx, launch)
	return err == nil && !running
}

func (s *ConnectionService) start(launch Launch, waitForConnection, retryInitial bool) error {
	if strings.TrimSpace(launch.RunID) == "" || !validEndpoint(launch.Endpoint) {
		return errors.New("run identity and valid agentctl endpoint are required")
	}
	ctx := s.ctx
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if _, exists := s.runs[launch.RunID]; exists {
		s.mu.Unlock()
		return nil
	}
	runCtx, cancelRun := context.WithCancel(ctx)
	s.runs[launch.RunID] = connectionRun{ctx: runCtx, cancel: cancelRun}
	s.mu.Unlock()

	var connected chan error
	if waitForConnection {
		connected = make(chan error, 1)
	}
	go func() {
		defer func() {
			s.mu.Lock()
			delete(s.runs, launch.RunID)
			s.mu.Unlock()
			cancelRun()
		}()
		first := true
		for {
			connectErr := s.client.Connect(runCtx, launch.Endpoint, launch.LastSequence, func() {
				if first {
					if connected != nil {
						connected <- nil
					}
					first = false
				}
			}, func(event interaction.Event) error {
				if launch.TaskSessionID != "" && (event.TaskSessionID != launch.TaskSessionID || event.OrganizationID != launch.OrganizationID || event.WorkspaceID != launch.WorkspaceID || event.TaskID != launch.TaskID) {
					return errors.New("agentctl event scope mismatch")
				}
				if event.RunID != launch.RunID {
					return errors.New("agentctl event run identity mismatch")
				}
				wrapped := Event{RunID: launch.RunID, Sequence: event.Sequence, Payload: event}
				if s.completion != nil {
					if err := s.completion.HandleEvent(runCtx, launch, wrapped); err != nil {
						return err
					}
				} else if err := s.repo.SaveEvent(runCtx, wrapped); err != nil {
					return err
				}
				launch.LastSequence = event.Sequence
				return nil
			})
			if errors.Is(connectErr, ErrAgentRunClosed) {
				cancelRun()
				return
			}
			if first {
				if retryInitial && runCtx.Err() == nil {
					if !waitForRetry(runCtx) {
						return
					}
					continue
				}
				if connected != nil {
					connected <- connectErr
				}
				if connectErr != nil || runCtx.Err() != nil {
					return
				}
				first = false
			}
			if runCtx.Err() != nil || !waitForRetry(runCtx) {
				return
			}
		}
	}()
	if connected == nil {
		return nil
	}
	select {
	case <-runCtx.Done():
		return runCtx.Err()
	case err := <-connected:
		return err
	}
}

func (s *ConnectionService) runContext(runID string) context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, exists := s.runs[runID]
	if !exists {
		return nil
	}
	return run.ctx
}

func (s *ConnectionService) cancelRun(runID string) {
	s.mu.Lock()
	run := s.runs[runID]
	s.mu.Unlock()
	if run.cancel != nil {
		run.cancel()
	}
}

func waitForRetry(ctx context.Context) bool {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *ConnectionService) reconcilePrompts(ctx context.Context, launch Launch) {
	if launch.TaskSessionID != "" {
		status, err := s.client.Status(ctx, launch.Endpoint)
		if err != nil || !matchesExecution(launch, status) {
			return
		}
	}
	outbox := s.repo
	prompts, err := outbox.PendingPrompts(ctx, launch.RunID)
	if err != nil {
		return
	}
	for _, prompt := range prompts {
		// Same identity and text are idempotent even if acceptance was recorded
		// before the previous connection disappeared.
		if err := s.client.Prompt(ctx, launch.Endpoint, prompt.CommandID, prompt.Text); err != nil {
			return
		}
		if err := outbox.PromptDelivered(ctx, launch.RunID, prompt.CommandID); err != nil {
			return
		}
	}
}
