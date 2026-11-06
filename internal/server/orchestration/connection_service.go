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

type AgentCommunication interface {
	Start(context.Context, orchestrationmodel.Launch) error
}

type connectionRun struct {
	ctx    context.Context
	cancel context.CancelFunc
	ready  chan struct{}
	done   chan struct{}
	err    error
}

type ConnectionService struct {
	ctx              context.Context
	client           AgentConnectionClient
	repo             ConnectionRepository
	completion       *LaunchCompletionService
	mu               sync.Mutex
	runs             map[string]*connectionRun
	background       *backgroundWork
	readinessTimeout time.Duration
	handshakes       chan struct{}
}

// ConnectionOption configures optional supervision policies.
type ConnectionOption func(*ConnectionService) error

func WithConnectionReadinessTimeout(timeout time.Duration) ConnectionOption {
	return func(service *ConnectionService) error {
		if timeout <= 0 {
			return errors.New("connection readiness timeout must be positive")
		}
		service.readinessTimeout = timeout
		return nil
	}
}

func NewConnectionService(ctx context.Context, client AgentConnectionClient, repo ConnectionRepository, completion *LaunchCompletionService, options ...ConnectionOption) (*ConnectionService, error) {
	if ctx == nil || client == nil || repo == nil {
		return nil, errors.New("agentctl client and launch repository are required")
	}
	background := newBackgroundWork(ctx)
	service := &ConnectionService{ctx: background.ctx, background: background, client: client, repo: repo, runs: make(map[string]*connectionRun), readinessTimeout: 10 * time.Second, handshakes: make(chan struct{}, 4)}
	service.completion = completion
	for _, option := range options {
		if err := option(service); err != nil {
			background.Close()
			return nil, err
		}
	}
	return service, nil
}

// Start performs an initial WebSocket handshake, then reconnects in the background.
func (s *ConnectionService) Start(ctx context.Context, launch orchestrationmodel.Launch) error {
	run, err := s.start(launch, initialConnection)
	if err != nil {
		return err
	}
	select {
	case <-run.ready:
		if run.err != nil {
			<-run.done
		}
		return run.err
	case <-ctx.Done():
		select {
		case <-run.ready:
			if run.err == nil {
				return nil
			}
		default:
		}
		run.cancel()
		<-run.done
		return ctx.Err()
	}
}

func (s *ConnectionService) Close() { s.background.Close() }

// ReconnectRecorded resumes event streams for persisted active launches. It never
// starts, configures, or replaces an agent process.
func (s *ConnectionService) ReconnectRecorded() error {
	launches, err := s.repo.ListReconnectableLaunches(s.ctx)
	if err != nil {
		return fmt.Errorf("load launches for reconnect: %w", err)
	}
	s.background.Go(func(ctx context.Context) {
		reconcileBatch(ctx, launches, func(ctx context.Context, launch orchestrationmodel.Launch) {
			if launch.Status == orchestrationmodel.LaunchCompletedPendingClose || launch.Status == orchestrationmodel.LaunchFailedPendingClose {
				return
			}
			if s.completion != nil && (launch.Status == orchestrationmodel.LaunchStarting || launch.Status == orchestrationmodel.LaunchFailed || strings.TrimSpace(launch.ProcessID) == "" || !validEndpoint(launch.Endpoint)) {
				status := terminalPendingStatus(launch)
				s.completion.CloseAbandoned(ctx, launch, status)
				return
			}
			if s.completion == nil {
				_, _ = s.start(launch, reconnectConnection)
				return
			}
			attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			s.reconcileAndReconnect(attemptCtx, launch)
		})
	})
	return nil
}

func terminalPendingStatus(launch orchestrationmodel.Launch) orchestrationmodel.LaunchStatus {
	if launch.LastEventType == string(orchestrationmodel.EventRunCompleted) || (launch.TaskSessionID == "" && launch.LastEventType == string(orchestrationmodel.EventPromptCompleted)) {
		return orchestrationmodel.LaunchCompletedPendingClose
	}
	return orchestrationmodel.LaunchFailedPendingClose
}

func (s *ConnectionService) reconcileAndReconnect(ctx context.Context, launch orchestrationmodel.Launch) {
	if launch.LastEventType == string(orchestrationmodel.EventRunCompleted) || (launch.TaskSessionID == "" && launch.LastEventType == string(orchestrationmodel.EventPromptCompleted)) {
		s.completion.CloseAbandoned(ctx, launch, orchestrationmodel.LaunchCompletedPendingClose)
		return
	}
	if launch.LastEventType == string(orchestrationmodel.EventRunFailed) || (launch.TaskSessionID == "" && launch.LastEventType == string(orchestrationmodel.EventPromptFailed)) {
		s.completion.CloseAbandoned(ctx, launch, orchestrationmodel.LaunchFailedPendingClose)
		return
	}
	statusCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	status, err := s.client.Status(statusCtx, launch.Endpoint)
	cancel()
	if err == nil && launch.TaskSessionID != "" && !matchesExecution(launch, status) {
		return
	}
	if err == nil && agentctlProcessHasEnded(status) {
		s.completion.CloseAbandoned(ctx, launch, orchestrationmodel.LaunchFailedPendingClose)
		return
	}
	if err != nil && s.containerConfirmedStopped(ctx, launch) {
		s.completion.CloseAbandoned(ctx, launch, orchestrationmodel.LaunchFailedPendingClose)
		return
	}
	if err := s.reconcilePrompts(ctx, launch); err != nil && ctx.Err() == nil {
		slog.Default().Warn("prompt reconciliation deferred", "run_id", launch.RunID, "workspace_id", launch.WorkspaceID, "reason", "delivery_unavailable")
	}
	_, _ = s.start(launch, reconnectConnection)
}

func agentctlProcessHasEnded(status orchestrationmodel.RunStatus) bool {
	if !status.Configured {
		return true
	}
	if status.TaskSessionID != "" {
		return false
	}
	if status.State == orchestrationmodel.StateFailed || status.State == orchestrationmodel.StateInterrupted || status.State == orchestrationmodel.StateExited {
		return true
	}
	if status.State == orchestrationmodel.StateStarting {
		return false
	}
	return status.Configured && (!status.ProcessAlive || !status.ExecutionAvailable)
}

func (s *ConnectionService) monitorLaunch(ctx context.Context, launch orchestrationmodel.Launch) {
	reconciliationFailed := false
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
				s.completion.CloseAbandoned(s.ctx, launch, orchestrationmodel.LaunchFailedPendingClose)
				return
			}
			continue
		}
		if launch.TaskSessionID != "" && !matchesExecution(launch, status) {
			continue
		}
		if !agentctlProcessHasEnded(status) {
			err := s.reconcilePrompts(ctx, launch)
			if err != nil && !reconciliationFailed && ctx.Err() == nil {
				slog.Default().Warn("prompt reconciliation deferred", "run_id", launch.RunID, "workspace_id", launch.WorkspaceID, "reason", "delivery_unavailable")
			}
			reconciliationFailed = err != nil
			continue
		}
		s.cancelRun(launch.RunID)
		s.completion.CloseAbandoned(s.ctx, launch, orchestrationmodel.LaunchFailedPendingClose)
		return
	}
}

func (s *ConnectionService) containerConfirmedStopped(ctx context.Context, launch orchestrationmodel.Launch) bool {
	inspectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	running, err := s.completion.AgentctlRunning(inspectCtx, launch)
	return err == nil && !running
}

func (s *ConnectionService) cancelRun(runID string) {
	s.mu.Lock()
	run := s.runs[runID]
	s.mu.Unlock()
	if run != nil {
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

func (s *ConnectionService) reconcilePrompts(ctx context.Context, launch orchestrationmodel.Launch) error {
	if launch.TaskSessionID != "" {
		status, err := s.client.Status(ctx, launch.Endpoint)
		if err != nil {
			return err
		}
		if !matchesExecution(launch, status) {
			return errors.New("agentctl execution identity mismatch")
		}
	}
	prompts, err := s.repo.PendingPrompts(ctx, launch.RunID)
	if err != nil {
		return err
	}
	for _, prompt := range prompts {
		// Same identity and text are idempotent even if acceptance was recorded
		// before the previous connection disappeared.
		if err := s.client.Prompt(ctx, launch.Endpoint, prompt.CommandID, prompt.Text); err != nil {
			return err
		}
		if err := s.repo.PromptDelivered(ctx, launch.RunID, prompt.CommandID); err != nil {
			return err
		}
	}
	return nil
}
