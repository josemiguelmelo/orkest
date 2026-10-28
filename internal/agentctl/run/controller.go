// Package run owns execution, conversation state, and durable recovery.
package run

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"time"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

var (
	ErrConfigurationConflict = errors.New("process is already configured for a run")
	ErrLaunchConflict        = errors.New("run launch already attempted or service closed")
	ErrNotConfigured         = errors.New("configure the agent before launching")
)

type RunStatus struct {
	OrganizationID     string
	WorkspaceID        string
	TaskID             string
	RunID              string
	RecoveryBlocked    bool
	Configured         bool
	ExecutionAvailable bool
	State              ExecutionState
	ProcessAlive       bool
	SessionID          string
	TaskSessionID      string
	ActiveCommandID    string
	QueueDepth         int
	QueuePaused        bool
	RunEnded           bool
	RunCancelled       bool
	RecoveryPermitted  bool
}

// RunController owns the lifecycle and conversation of one execution run.
// Configuring a run never starts work.
type RunController struct {
	recoveryBlocked bool
	// mu protects configuration and session lifecycle. When needed, acquire it
	// before the conversation lock; conversation operations never acquire this lock.
	mu              sync.RWMutex
	runConfig       *runconfig.RunConfig
	host            agentruntime.RuntimeHost
	stateDir        string
	ctx             context.Context
	cancel          context.CancelFunc
	session         agentruntime.RuntimeSession
	checkpoints     checkpointStore
	launchAttempted bool
	closed          bool
	launchState     ExecutionState
	startupWorkers  sync.WaitGroup
	restoreMu       sync.Mutex
	conversation    *conversation
	logger          *slog.Logger
}

// NewRunController wires the runtime and durable storage at the application boundary.
func NewRunController(ctx context.Context, host agentruntime.RuntimeHost, stateDir string, logger *slog.Logger) *RunController {
	ctx, cancel := context.WithCancel(ctx)
	if logger == nil {
		logger = slog.Default()
	}
	return &RunController{
		ctx: ctx, cancel: cancel, host: host, stateDir: stateDir,
		conversation: newConversation(ctx, stateDir, logger),
		checkpoints:  checkpointStore{StateDir: stateDir}, logger: logger,
	}
}

// Configure accepts identical resubmissions and atomically rejects changed bindings.
func (c *RunController) Configure(config runconfig.RunConfig) error {
	config = config.Clone()
	if err := config.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.host != nil && config.Runtime != c.host.Runtime() {
		return agentruntime.ErrRuntimeHostUnavailable
	}
	if c.runConfig != nil && !reflect.DeepEqual(*c.runConfig, config) {
		current := c.runConfig.Clone()
		candidate := config.Clone()
		current.GitCredentials, candidate.GitCredentials = nil, nil
		if !reflect.DeepEqual(current, candidate) || len(c.runConfig.GitCredentials) != 0 || len(config.GitCredentials) == 0 {
			return ErrConfigurationConflict
		}
		if err := c.checkpoints.Save(config); err != nil {
			return errors.New("persist run configuration")
		}
		c.runConfig = &config
		return nil
	}
	c.runConfig = &config
	c.logger.Info("agent run configured", runLogAttrs(config)...)
	return nil
}

func (c *RunController) Status() RunStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	status := RunStatus{
		RecoveryBlocked:    c.recoveryBlocked,
		Configured:         c.runConfig != nil,
		ExecutionAvailable: c.host != nil && !c.closed,
		State:              c.launchState,
	}
	if c.runConfig != nil {
		status.TaskSessionID = c.runConfig.TaskSessionID
		status.OrganizationID = c.runConfig.OrganizationID
		status.WorkspaceID = c.runConfig.WorkspaceID
		status.TaskID = c.runConfig.TaskID
		status.RunID = c.runConfig.RunID
	}
	conversation := c.conversation.Snapshot()
	status.ActiveCommandID = conversation.ActiveCommandID
	status.QueueDepth = conversation.QueueDepth
	status.QueuePaused = conversation.QueuePaused
	status.RunEnded = conversation.RunEnded
	status.RunCancelled = conversation.Cancelled
	status.RecoveryPermitted = conversation.RecoveryAllowed
	if c.session != nil {
		status.SessionID = c.session.SessionID()
		status.State, status.ProcessAlive = observedExecutionState(conversation.State, c.session)
	}
	return status
}

// CanAutoRestore keeps recovery policy with the controller; the daemon
// only schedules periodic supervision.
func (c *RunController) CanAutoRestore() bool {
	status := c.Status()
	c.mu.RLock()
	credentialsReady := c.runConfig != nil && (!c.runConfig.GitCredentialsRequired || len(c.runConfig.GitCredentials) > 0)
	c.mu.RUnlock()
	return credentialsReady && !status.RecoveryBlocked && status.RecoveryPermitted && !status.RunCancelled && !status.RunEnded && status.TaskSessionID != "" && status.SessionID != "" &&
		!status.ProcessAlive && (status.State == StateExited || status.State == StateFailed || status.State == StateInterrupted)
}

// RestoreSession serializes replacement of a terminated runtime session.
func (c *RunController) RestoreSession() error {
	if !c.restoreMu.TryLock() {
		return ErrLaunchConflict
	}
	defer c.restoreMu.Unlock()
	c.mu.Lock()
	if c.runConfig != nil && c.runConfig.GitCredentialsRequired && len(c.runConfig.GitCredentials) == 0 {
		c.mu.Unlock()
		return ErrNotConfigured
	}
	if c.closed || c.launchState == StateStarting {
		c.mu.Unlock()
		return ErrLaunchConflict
	}
	if c.conversation.Snapshot().RunEnded {
		c.mu.Unlock()
		return ErrLaunchConflict
	}
	var oldConversation *conversation
	if c.launchAttempted && c.session != nil {
		select {
		case <-c.session.Done():
			oldConversation = c.conversation
			c.conversation = newConversation(c.ctx, c.stateDir, c.logger)
			c.session = nil
		default:
			c.mu.Unlock()
			return ErrLaunchConflict
		}
	}
	if c.launchAttempted && c.session == nil {
		c.launchAttempted = false
	}
	if c.runConfig == nil || c.host == nil {
		c.mu.Unlock()
		return ErrNotConfigured
	}
	c.recoveryBlocked = false
	c.launchAttempted, c.launchState = true, StateStarting
	config := *c.runConfig
	c.startupWorkers.Add(1)
	c.mu.Unlock()
	defer c.startupWorkers.Done()
	if oldConversation != nil {
		_ = oldConversation.close()
	}
	return c.startReservedSession(restoreSession, config)
}

type sessionStartMode uint8

const (
	createSession sessionStartMode = iota
	restoreSession
)

// Launch reserves a single attempt before startup I/O. Failed attempts are
// never silently retried; startup belongs to the controller context.
func (c *RunController) Launch() error {
	c.mu.Lock()
	if c.runConfig == nil {
		c.mu.Unlock()
		return ErrNotConfigured
	}
	if c.launchAttempted || c.closed {
		c.mu.Unlock()
		return ErrLaunchConflict
	}
	if c.host == nil {
		c.mu.Unlock()
		return agentruntime.ErrRuntimeHostUnavailable
	}
	c.launchAttempted = true
	c.launchState = StateStarting
	config := *c.runConfig
	c.startupWorkers.Add(1)
	c.mu.Unlock()
	defer c.startupWorkers.Done()
	return c.startReservedSession(createSession, config)
}

func (c *RunController) startReservedSession(mode sessionStartMode, config runconfig.RunConfig) error {
	startedAt := time.Now()
	c.logger.Info("runtime launch started", runLogAttrs(config)...)
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	var session agentruntime.RuntimeSession
	var err error
	if mode == restoreSession {
		session, err = c.host.Restore(ctx, config.ForRuntime())
	} else {
		session, err = c.host.Launch(ctx, config.ForRuntime())
	}
	c.mu.Lock()
	if err != nil {
		if c.closed {
			c.mu.Unlock()
			return nil
		}
		c.launchState = StateFailed
		if mode == restoreSession {
			c.launchState = StateInterrupted
			c.recoveryBlocked = true
		}
		c.logger.Error("runtime launch failed", append(runLogAttrs(config), "error_type", fmt.Sprintf("%T", err), "duration_ms", time.Since(startedAt).Milliseconds())...)
		c.mu.Unlock()
		return err
	}
	if c.closed {
		c.mu.Unlock()
		_ = session.Close()
		return nil
	}
	c.session = session
	c.conversation.bind(config, session)
	conversation := c.conversation
	c.mu.Unlock()
	if config.TaskSessionID != "" {
		if c.checkpoints.Save(config) != nil {
			err = ErrJournal
		}
	}
	if err == nil && mode == restoreSession {
		err = conversation.ReplayAndReconcile(ctx)
	}
	if err != nil {
		_ = session.Close()
		conversation.setExecutionState(StateInterrupted)
		c.mu.Lock()
		c.launchState, c.recoveryBlocked = StateInterrupted, true
		c.mu.Unlock()
		return err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		_ = session.Close()
		return nil
	}
	c.launchState = StateReady
	c.logger.Info("runtime session ready", append(runLogAttrs(config), "session_id", session.SessionID(), "duration_ms", time.Since(startedAt).Milliseconds())...)
	c.mu.Unlock()
	return nil
}

// Close terminates the owned provider only when agentctl itself shuts down.
func (c *RunController) Close() error {
	c.mu.Lock()
	c.closed = true
	c.conversation.stop()
	c.cancel()
	session := c.session
	c.mu.Unlock()
	if session != nil {
		_ = session.Close()
	}
	c.startupWorkers.Wait()
	return c.conversation.close()
}

// Prompt delegates conversation commands without coupling them to transport lifetimes.
func (c *RunController) Prompt(commandID, text string) error {
	return c.currentConversation().Prompt(commandID, text)
}

func (c *RunController) CancelPrompt(commandID string) error {
	return c.currentConversation().CancelPrompt(commandID)
}

func (c *RunController) RespondPermission(response interaction.PermissionResponse) error {
	return c.currentConversation().RespondPermission(response)
}

func (c *RunController) Subscribe(after uint64) ([]interaction.Event, <-chan interaction.Event, func(), error) {
	return c.currentConversation().Subscribe(after)
}

func runLogAttrs(config runconfig.RunConfig) []any {
	return []any{"organization_id", config.OrganizationID, "workspace_id", config.WorkspaceID, "task_id", config.TaskID, "run_id", config.RunID, "runtime", config.Runtime}
}

// RecoverFromCheckpoint restores the last bound run on daemon restart. Runtime ownership must
// be acquired before any prompt is admitted. Finished runs are never resumed.
func (c *RunController) RecoverFromCheckpoint(expectedRunID string) error {
	config, found, err := c.checkpoints.Load(expectedRunID)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if err := c.Configure(config); err != nil {
		return err
	}
	if config.GitCredentialsRequired && len(config.GitCredentials) == 0 {
		return nil
	}
	if config.TaskSessionID != "" {
		ended, err := journalRunEnded(c.stateDir, config)
		if err != nil {
			return err
		}
		if ended {
			c.mu.Lock()
			c.launchAttempted, c.launchState = true, StateExited
			c.conversation.markRecoveredRunEnded()
			c.mu.Unlock()
			return nil
		}
	}
	return c.RestoreSession()
}

func (c *RunController) currentConversation() *conversation {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conversation
}

func (c *RunController) CommandStatus(commandID string) (interaction.CommandStatus, error) {
	return c.currentConversation().CommandStatus(commandID)
}

func (c *RunController) ResumeQueue() error { return c.currentConversation().ResumeQueue() }

func (c *RunController) DiscardQueue() error { return c.currentConversation().DiscardQueue() }
