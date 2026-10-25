// Package agentctl owns run configuration and execution state independently of transport.
package agentctl

import (
	"context"
	"errors"
	"sync"
)

var ErrConfigurationConflict = errors.New("process is already configured for a run")

type Status struct {
	Configured         bool
	ExecutionAvailable bool
	State              string
	SessionID          string
}

// Service owns process-local run binding. Configuration never starts work.
type Service struct {
	mu            sync.RWMutex
	configuration *Configuration
	provider      Provider
	ctx           context.Context
	cancel        context.CancelFunc
	process       Process
	attempted     bool
	closed        bool
	state         string
	launches      sync.WaitGroup
}

func NewService() *Service { return &Service{} }

// NewServiceWithProvider wires a runtime at the application boundary.
func NewServiceWithProvider(ctx context.Context, provider Provider) *Service {
	ctx, cancel := context.WithCancel(ctx)
	return &Service{ctx: ctx, cancel: cancel, provider: provider}
}

// Configure accepts identical resubmissions and atomically rejects changed bindings.
func (s *Service) Configure(config Configuration) error {
	if err := config.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.provider != nil && config.Runtime != s.provider.Runtime() {
		return ErrProviderUnavailable
	}
	if s.configuration != nil && *s.configuration != config {
		return ErrConfigurationConflict
	}
	s.configuration = &config
	return nil
}

func (s *Service) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	status := Status{Configured: s.configuration != nil, ExecutionAvailable: s.provider != nil && !s.closed, State: s.state}
	if s.process != nil {
		status.SessionID = s.process.SessionID()
		select {
		case <-s.process.Done():
			status.State = "exited"
		default:
		}
	}
	return status
}
