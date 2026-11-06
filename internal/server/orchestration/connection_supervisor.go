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

type connectionMode uint8

const (
	initialConnection connectionMode = iota
	reconnectConnection
)

func (s *ConnectionService) start(launch orchestrationmodel.Launch, mode connectionMode) (*connectionRun, error) {
	if strings.TrimSpace(launch.RunID) == "" || !validEndpoint(launch.Endpoint) {
		return nil, errors.New("run identity and valid agentctl endpoint are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return nil, context.Canceled
	}
	if run := s.runs[launch.RunID]; run != nil {
		return run, nil
	}
	runCtx, cancel := context.WithCancel(s.ctx)
	run := &connectionRun{ctx: runCtx, cancel: cancel, ready: make(chan struct{}), done: make(chan struct{})}
	s.runs[launch.RunID] = run
	if !s.background.Go(func(context.Context) { s.supervise(run, launch, mode) }) {
		delete(s.runs, launch.RunID)
		cancel()
		return nil, context.Canceled
	}
	return run, nil
}

func (s *ConnectionService) supervise(run *connectionRun, launch orchestrationmodel.Launch, mode connectionMode) {
	var monitor sync.WaitGroup
	notified := false
	notify := func(err error) {
		if !notified {
			run.err = err
			notified = true
			close(run.ready)
		}
	}
	defer func() {
		run.cancel()
		monitor.Wait()
		notify(run.ctx.Err())
		s.mu.Lock()
		if s.runs[launch.RunID] == run {
			delete(s.runs, launch.RunID)
		}
		s.mu.Unlock()
		close(run.done)
	}()
	monitoring := false
	startMonitor := func() {
		if !monitoring && s.completion != nil {
			monitoring = true
			observed := launch
			monitor.Go(func() { s.monitorLaunch(run.ctx, observed) })
		}
	}
	if mode == reconnectConnection {
		startMonitor()
	}
	disconnected := false
	first := true
	for run.ctx.Err() == nil {
		err := s.connectAttempt(run.ctx, launch, func() {
			first = false
			notify(nil)
			if disconnected {
				slog.Default().Info("agent activity reconnected", "run_id", launch.RunID, "workspace_id", launch.WorkspaceID)
				disconnected = false
			}
			startMonitor()
		}, func(event orchestrationmodel.AgentActivityEvent) error {
			if launch.TaskSessionID != "" && (event.TaskSessionID != launch.TaskSessionID || event.OrganizationID != launch.OrganizationID || event.WorkspaceID != launch.WorkspaceID || event.TaskID != launch.TaskID) {
				return errors.New("agentctl event scope mismatch")
			}
			if event.RunID != launch.RunID {
				return errors.New("agentctl event run identity mismatch")
			}
			wrapped := orchestrationmodel.Event{RunID: launch.RunID, Sequence: event.Sequence, Payload: event}
			if s.completion != nil {
				if err := s.completion.HandleEvent(run.ctx, launch, wrapped); err != nil {
					return err
				}
			} else if err := s.repo.SaveEvent(run.ctx, wrapped); err != nil {
				return err
			}
			launch.LastSequence = event.Sequence
			return nil
		})
		if errors.Is(err, ErrAgentRunClosed) {
			return
		}
		if first && mode == initialConnection {
			if err == nil {
				err = errors.New("agentctl stream ended before readiness")
			}
			notify(err)
			return
		}
		if run.ctx.Err() == nil && !disconnected {
			slog.Default().Warn("agent activity disconnected", "run_id", launch.RunID, "workspace_id", launch.WorkspaceID, "reason", "stream_unavailable")
			disconnected = true
		}
		if !waitForRetry(run.ctx) {
			return
		}
	}
}

// The handshake timer cancels only an unready attempt. Once ready, its context
// remains owned by supervision rather than the caller or startup deadline.
func (s *ConnectionService) connectAttempt(ctx context.Context, launch orchestrationmodel.Launch, ready func(), event func(orchestrationmodel.AgentActivityEvent) error) error {
	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var mu sync.Mutex
	established, expired := false, false
	timer := time.AfterFunc(s.readinessTimeout, func() {
		mu.Lock()
		defer mu.Unlock()
		if !established {
			expired = true
			cancel()
		}
	})
	defer timer.Stop()
	select {
	case s.handshakes <- struct{}{}:
	case <-attemptCtx.Done():
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("agentctl readiness: %w", context.DeadlineExceeded)
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { <-s.handshakes }) }
	defer release()
	err := s.client.Connect(attemptCtx, launch.Endpoint, launch.LastSequence, func() {
		mu.Lock()
		if expired {
			mu.Unlock()
			return
		}
		established = true
		timer.Stop()
		mu.Unlock()
		release()
		ready()
	}, event)
	mu.Lock()
	defer mu.Unlock()
	if expired {
		return fmt.Errorf("agentctl readiness: %w", context.DeadlineExceeded)
	}
	return err
}
