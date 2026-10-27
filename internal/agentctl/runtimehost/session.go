package runtimehost

import (
	"context"
	"os"
	"strings"
	"sync"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

// Session owns the child process, protocol connection, and task ownership lease.
// Host creates sessions after validating and provisioning their runtime.
type Session struct {
	child       *childProcess
	connection  agentruntime.ProtocolClient
	callbacks   *sessionCallbacks
	sessionID   string
	promptMu    sync.Mutex
	ownership   *os.File
	releaseOnce sync.Once
}

func (s *Session) SessionID() string { return s.sessionID }

func (s *Session) Done() <-chan struct{} { return s.child.Done() }

func (s *Session) Err() error {
	if err := s.child.Err(); err != nil {
		return err
	}
	return s.connection.Err()
}

// Close releases the task lease only after the child has terminated.
func (s *Session) Close() error {
	_ = s.child.Close()
	s.releaseOwnership()
	return nil
}

func (s *Session) releaseOwnership() {
	s.releaseOnce.Do(func() { _ = s.ownership.Close() })
}

func (s *Session) supervise() {
	select {
	case <-s.child.Done():
		s.releaseOwnership()
	case <-s.connection.Done():
		_ = s.Close()
	}
}

func (s *Session) Prompt(ctx context.Context, command interaction.Command, observer agentruntime.PromptObserver) (agentruntime.StopReason, error) {
	if strings.TrimSpace(command.ID) == "" || strings.TrimSpace(command.Text) == "" {
		return "", interaction.ErrCommand
	}
	s.promptMu.Lock()
	defer s.promptMu.Unlock()
	promptContext, cancel := context.WithCancel(ctx)
	defer cancel()
	s.callbacks.attachObserver(promptContext, cancel, observer)
	defer s.callbacks.detachObserver()
	reason, err := s.connection.Prompt(promptContext, s.sessionID, interaction.EncodeCommandPrompt(command.ID, command.Text))
	if err != nil {
		observerFailed := s.callbacks.hasObserverFailure()
		select {
		case <-s.connection.Done():
			_ = s.Close()
		default:
			if observerFailed {
				_ = s.Close()
			}
		}
		return "", err
	}
	return reason, nil
}

func (s *Session) Cancel(ctx context.Context) error {
	return s.connection.Cancel(ctx, s.sessionID)
}

func (s *Session) Reconcile(ctx context.Context, command interaction.Command) (agentruntime.RecoveryDecision, error) {
	if err := ctx.Err(); err != nil {
		return agentruntime.RecoveryDecision{}, err
	}
	return s.callbacks.recoveryDecision(command), nil
}

var _ agentruntime.RuntimeSession = (*Session)(nil)
