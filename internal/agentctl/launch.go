package agentctl

import (
	"context"
	"errors"
	"time"
)

var (
	ErrLaunchConflict      = errors.New("run launch already attempted or service closed")
	ErrNotConfigured       = errors.New("configure the agent before launching")
	ErrProviderUnavailable = errors.New("agent provider unavailable")
)

// Launch reserves the attempt before I/O. Failed attempts are never silently retried.
// The service context, rather than the requesting connection, owns startup.
func (s *Service) Launch() error {
	s.mu.Lock()
	if s.configuration == nil {
		s.mu.Unlock()
		return ErrNotConfigured
	}
	if s.attempted || s.closed {
		s.mu.Unlock()
		return ErrLaunchConflict
	}
	if s.provider == nil {
		s.mu.Unlock()
		return ErrProviderUnavailable
	}
	s.attempted = true
	s.state = "starting"
	config := *s.configuration
	s.launches.Add(1)
	s.mu.Unlock()
	defer s.launches.Done()
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()
	process, err := s.provider.Launch(ctx, config)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.state = "failed"
		return err
	}
	s.process = process
	s.state = "ready"
	return nil
}

// Close terminates the owned provider only when agentctl itself shuts down.
func (s *Service) Close() error {
	s.mu.Lock()
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.launches.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.process != nil {
		return s.process.Close()
	}
	return nil
}
