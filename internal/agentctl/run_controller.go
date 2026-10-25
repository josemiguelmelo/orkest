// Package agentctl owns run configuration and execution state independently of transport.
package agentctl

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrConfigurationConflict = errors.New("process is already configured for a run")
	ErrLaunchConflict        = errors.New("run launch already attempted or service closed")
	ErrNotConfigured         = errors.New("configure the agent before launching")
	ErrProviderUnavailable   = errors.New("agent provider unavailable")
)

// LaunchError carries a provider-sanitized message suitable for API responses.
// Arbitrary provider errors remain internal and are mapped to a generic response.
type LaunchError struct {
	Message string
	Cause   error
}

func (e *LaunchError) Error() string { return e.Message }
func (e *LaunchError) Unwrap() error { return e.Cause }

type Status struct {
	Configured         bool
	ExecutionAvailable bool
	State              State
	ProcessAlive       bool
	SessionID          string
}

// RunController owns the lifecycle and conversation of one execution run.
// Configuring a run never starts work.
type RunController struct {
	// mu protects configuration and process lifecycle. When needed, acquire it
	// before the conversation lock; conversation operations never acquire this lock.
	mu           sync.RWMutex
	runConfig    *RunConfig
	provider     Provider
	ctx          context.Context
	cancel       context.CancelFunc
	process      Process
	attempted    bool
	closed       bool
	launchState  State
	launches     sync.WaitGroup
	conversation *ConversationService
}

func NewRunController() *RunController {
	return NewRunControllerWithProvider(context.Background(), nil, "")
}

// NewRunControllerWithProvider wires a runtime at the application boundary.
func NewRunControllerWithProvider(ctx context.Context, provider Provider, stateDir string) *RunController {
	ctx, cancel := context.WithCancel(ctx)
	return &RunController{
		ctx:          ctx,
		cancel:       cancel,
		provider:     provider,
		conversation: newConversationService(ctx, stateDir),
	}
}

// Configure accepts identical resubmissions and atomically rejects changed bindings.
func (c *RunController) Configure(config RunConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.provider != nil && config.Runtime != c.provider.Runtime() {
		return ErrProviderUnavailable
	}
	if c.runConfig != nil && *c.runConfig != config {
		return ErrConfigurationConflict
	}
	c.runConfig = &config
	return nil
}

func (c *RunController) Status() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	status := Status{Configured: c.runConfig != nil, ExecutionAvailable: c.provider != nil && !c.closed, State: c.launchState}
	if c.process != nil {
		status.State = c.conversation.stateSnapshot()
		status.SessionID = c.process.SessionID()
		select {
		case <-c.process.Done():
			if status.State != StateInterrupted && status.State != StateFailed {
				status.State = StateExited
				if c.process.Err() != nil {
					status.State = StateFailed
				}
			}
		default:
			status.ProcessAlive = true
		}
	}
	return status
}

// Launch reserves the attempt before I/O. Failed attempts are never silently retried.
// The controller context, rather than the requesting connection, owns startup.
func (c *RunController) Launch() error {
	c.mu.Lock()
	if c.runConfig == nil {
		c.mu.Unlock()
		return ErrNotConfigured
	}
	if c.attempted || c.closed {
		c.mu.Unlock()
		return ErrLaunchConflict
	}
	if c.provider == nil {
		c.mu.Unlock()
		return ErrProviderUnavailable
	}
	c.attempted = true
	c.launchState = StateStarting
	config := *c.runConfig
	c.launches.Add(1)
	c.mu.Unlock()
	defer c.launches.Done()
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	process, err := c.provider.Launch(ctx, config)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.launchState = StateFailed
		return err
	}
	c.process = process
	c.conversation.bind(config, process)
	c.launchState = StateReady
	return nil
}

// Close terminates the owned provider only when agentctl itself shuts down.
func (c *RunController) Close() error {
	c.mu.Lock()
	c.closed = true
	c.conversation.stop()
	if c.cancel != nil {
		c.cancel()
	}
	process := c.process
	c.mu.Unlock()
	if process != nil {
		_ = process.Close()
	}
	c.launches.Wait()
	c.mu.RLock()
	launched := c.process
	c.mu.RUnlock()
	if launched != nil && process == nil {
		_ = launched.Close()
	}
	return c.conversation.close()
}

// Prompt delegates conversation commands without coupling them to transport lifetimes.
func (c *RunController) Prompt(commandID, text string) error {
	return c.conversation.Prompt(commandID, text)
}

func (c *RunController) CancelPrompt(commandID string) error {
	return c.conversation.CancelPrompt(commandID)
}

func (c *RunController) RespondPermission(response PermissionResponse) error {
	return c.conversation.RespondPermission(response)
}

func (c *RunController) Subscribe(after uint64) ([]Event, <-chan Event, func(), error) {
	return c.conversation.Subscribe(after)
}
