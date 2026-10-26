// Package agentctl owns run configuration and execution state independently of transport.
package agentctl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"
)

var (
	ErrConfigurationConflict      = errors.New("process is already configured for a run")
	ErrLaunchConflict             = errors.New("run launch already attempted or service closed")
	ErrNotConfigured              = errors.New("configure the agent before launching")
	ErrRuntimeProviderUnavailable = errors.New("agent provider unavailable")
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
	OrganizationID     string
	WorkspaceID        string
	TaskID             string
	RunID              string
	RecoveryBlocked    bool
	Configured         bool
	ExecutionAvailable bool
	State              State
	ProcessAlive       bool
	SessionID          string
	TaskSessionID      string
	ActiveCommand      string
	QueueDepth         int
	QueuePaused        bool
	RunEnded           bool
}

// RunController owns the lifecycle and conversation of one execution run.
// Configuring a run never starts work.
type RunController struct {
	recoveryBlocked bool
	// mu protects configuration and process lifecycle. When needed, acquire it
	// before the conversation lock; conversation operations never acquire this lock.
	mu           sync.RWMutex
	runConfig    *RunConfig
	provider     RuntimeProvider
	ctx          context.Context
	cancel       context.CancelFunc
	process      Process
	attempted    bool
	closed       bool
	launchState  State
	launches     sync.WaitGroup
	conversation *ConversationService
	logger       *slog.Logger
}

func NewRunController() *RunController {
	return NewRunControllerWithRuntimeProvider(context.Background(), nil, "")
}

// NewRunControllerWithRuntimeProvider wires a runtime at the application boundary.
func NewRunControllerWithRuntimeProvider(ctx context.Context, provider RuntimeProvider, stateDir string, loggers ...*slog.Logger) *RunController {
	ctx, cancel := context.WithCancel(ctx)
	logger := slog.Default()
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	return &RunController{
		ctx:          ctx,
		cancel:       cancel,
		provider:     provider,
		conversation: newConversationService(ctx, stateDir, logger),
		logger:       logger,
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
		return ErrRuntimeProviderUnavailable
	}
	if c.runConfig != nil && !reflect.DeepEqual(*c.runConfig, config) {
		return ErrConfigurationConflict
	}
	c.runConfig = &config
	c.logger.Info("agent run configured", runLogAttrs(config)...)
	return nil
}

func (c *RunController) Status() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	status := Status{RecoveryBlocked: c.recoveryBlocked, Configured: c.runConfig != nil, ExecutionAvailable: c.provider != nil && !c.closed, State: c.launchState}
	if c.runConfig != nil {
		status.TaskSessionID = c.runConfig.TaskSessionID
		status.OrganizationID, status.WorkspaceID, status.TaskID, status.RunID = c.runConfig.OrganizationID, c.runConfig.WorkspaceID, c.runConfig.TaskID, c.runConfig.RunID
	}
	c.conversation.mu.RLock()
	status.ActiveCommand, status.QueueDepth, status.QueuePaused, status.RunEnded = c.conversation.activeCommand, len(c.conversation.queue), c.conversation.paused, c.conversation.runEnded
	c.conversation.mu.RUnlock()
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
func (c *RunController) Launch() error { return c.launch(false) }

func (c *RunController) Restore() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrLaunchConflict
	}
	if c.attempted && c.process == nil && c.launchState != StateStarting {
		c.attempted = false
	}
	c.recoveryBlocked = false
	if c.attempted && c.process != nil {
		select {
		case <-c.process.Done():
			old := c.conversation
			_ = old.close()
			c.conversation = newConversationService(c.ctx, old.stateDir, c.logger)
			c.process, c.attempted = nil, false
		default:
			c.mu.Unlock()
			return ErrLaunchConflict
		}
	}
	c.mu.Unlock()
	return c.launch(true)
}

func (c *RunController) launch(restore bool) error {
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
		return ErrRuntimeProviderUnavailable
	}
	c.attempted = true
	c.launchState = StateStarting
	config := *c.runConfig
	c.launches.Add(1)
	c.mu.Unlock()
	defer c.launches.Done()
	startedAt := time.Now()
	c.logger.Info("runtime launch started", runLogAttrs(config)...)
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	var process Process
	var err error
	if restore {
		provider, ok := c.provider.(RestoringRuntimeProvider)
		if !ok {
			err = ErrRuntimeProviderUnavailable
		} else {
			process, err = provider.Restore(ctx, config)
		}
	} else {
		process, err = c.provider.Launch(ctx, config)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.launchState = StateFailed
		if restore {
			c.launchState = StateInterrupted
			c.recoveryBlocked = true
		}
		c.logger.Error("runtime launch failed", append(runLogAttrs(config), "error_type", fmt.Sprintf("%T", err), "duration_ms", time.Since(startedAt).Milliseconds())...)
		return err
	}
	c.process = process
	c.conversation.bind(config, process)
	if config.TaskSessionID != "" {
		data, encodeErr := json.Marshal(runCheckpoint{Version: 1, Config: config})
		if encodeErr == nil {
			encodeErr = WriteDurableFile(filepath.Join(c.conversation.stateDir, "current-run.json"), data)
		}
		if encodeErr != nil {
			_ = process.Close()
			c.launchState, c.recoveryBlocked = StateInterrupted, true
			c.conversation.mu.Lock()
			c.conversation.state = StateInterrupted
			c.conversation.mu.Unlock()
			return ErrJournal
		}
	}
	if restore {
		if err := c.conversation.restoreConversation(); err != nil {
			_ = process.Close()
			c.launchState, c.recoveryBlocked = StateInterrupted, true
			c.conversation.mu.Lock()
			c.conversation.state = StateInterrupted
			c.conversation.mu.Unlock()
			return err
		}
	}
	c.launchState = StateReady
	c.logger.Info("runtime session ready", append(runLogAttrs(config), "session_id", process.SessionID(), "duration_ms", time.Since(startedAt).Milliseconds())...)
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
	return c.conversationSnapshot().Prompt(commandID, text)
}

func (c *RunController) CancelPrompt(commandID string) error {
	return c.conversationSnapshot().CancelPrompt(commandID)
}

func (c *RunController) RespondPermission(response PermissionResponse) error {
	return c.conversationSnapshot().RespondPermission(response)
}

func (c *RunController) Subscribe(after uint64) ([]Event, <-chan Event, func(), error) {
	return c.conversationSnapshot().Subscribe(after)
}

func runLogAttrs(config RunConfig) []any {
	return []any{"organization_id", config.OrganizationID, "workspace_id", config.WorkspaceID, "task_id", config.TaskID, "run_id", config.RunID, "runtime", config.Runtime}
}

// Recover restores the last bound run on daemon restart. Runtime ownership must
// be acquired before any prompt is admitted. Finished runs are never resumed.
func (c *RunController) Recover(expectedRuns ...string) error {
	path := filepath.Join(c.conversation.stateDir, "current-run.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return ErrJournal
	}
	var checkpoint runCheckpoint
	if json.Unmarshal(data, &checkpoint) != nil || checkpoint.Version != 1 {
		return ErrJournal
	}
	config := checkpoint.Config
	if len(expectedRuns) > 0 && expectedRuns[0] != "" && config.RunID != expectedRuns[0] {
		return nil
	}
	if err := c.Configure(config); err != nil {
		return err
	}
	return c.Restore()
}

func (c *RunController) conversationSnapshot() *ConversationService {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conversation
}

type runCheckpoint struct {
	Version int       `json:"version"`
	Config  RunConfig `json:"config"`
}
