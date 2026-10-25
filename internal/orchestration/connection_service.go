package orchestration

import (
	"context"
	"errors"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

type AgentCommunication interface {
	Start(string, string, uint64) error
}

type ConnectionService struct {
	ctx    context.Context
	client AgentctlClient
	repo   LaunchRepository
}

func NewConnectionService(ctx context.Context, client AgentctlClient, repo LaunchRepository) (*ConnectionService, error) {
	if ctx == nil || client == nil || repo == nil {
		return nil, errors.New("agentctl client and launch repository are required")
	}
	return &ConnectionService{ctx: ctx, client: client, repo: repo}, nil
}

// Start performs an initial WebSocket handshake, then reconnects in the background.
func (s *ConnectionService) Start(runID, endpoint string, cursor uint64) error {
	ctx := s.ctx
	connected := make(chan error, 1)
	go func() {
		first := true
		for {
			connectErr := s.client.Connect(ctx, endpoint, cursor, func() {
				if first {
					connected <- nil
					first = false
				}
			}, func(event agentctl.Event) error {
				if event.RunID != runID {
					return errors.New("agentctl event run identity mismatch")
				}
				if err := s.repo.SaveEvent(ctx, Event{RunID: runID, Sequence: event.Sequence, Payload: event}); err != nil {
					return err
				}
				cursor = event.Sequence
				return nil
			})
			if first {
				connected <- connectErr
				first = false
				if connectErr != nil {
					return
				}
			}
			if ctx.Err() != nil {
				return
			}
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-connected:
		return err
	}
}
