// Package agentctl owns run configuration and execution state independently of transport.
package agentctl

import (
	"errors"
	"sync"
)

var ErrConfigurationConflict = errors.New("process is already configured for a run")

type Status struct {
	Configured         bool
	ExecutionAvailable bool
}

// Service owns process-local run binding. Configuration never starts work.
type Service struct {
	mu            sync.RWMutex
	configuration *Configuration
}

func NewService() *Service { return &Service{} }

// Configure accepts identical resubmissions and atomically rejects changed bindings.
func (s *Service) Configure(config Configuration) error {
	if err := config.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.configuration != nil && *s.configuration != config {
		return ErrConfigurationConflict
	}
	s.configuration = &config
	return nil
}

func (s *Service) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Status{Configured: s.configuration != nil, ExecutionAvailable: false}
}
