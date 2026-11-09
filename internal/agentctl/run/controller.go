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

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	"github.com/josemiguelmelo/orkest/internal/agentctl/repositoryobserver"
	runmodel "github.com/josemiguelmelo/orkest/internal/agentctl/run/model"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

var (
	ErrConfigurationConflict = errors.New("process is already configured for a run")
	ErrLaunchConflict        = errors.New("run launch already attempted or service closed")
	ErrRecoveryBlocked       = errors.New("saved run requires recovery reconciliation")
	ErrNotConfigured         = errors.New("configure the agent before launching")
)

// RunController owns the lifecycle and conversation of one execution run.
// Configuring a run never starts work.
type RunController struct {
	recoveryBlocked   bool
	checkpointBlocked bool
	checkpointMu      sync.Mutex
	// mu protects configuration and session lifecycle. When needed, acquire it
	// before the conversation lock; conversation operations never acquire this lock.
	mu              sync.RWMutex
	runConfig       *configmodel.RunConfig
	host            agentruntime.RuntimeHost
	stateDir        string
	ctx             context.Context
	cancel          context.CancelFunc
	session         agentruntime.RuntimeSession
	checkpoints     checkpointStore
	launchAttempted bool
	closed          bool
	launchState     runmodel.ExecutionState
	startupWorkers  sync.WaitGroup
	restoreMu       sync.Mutex
	conversation    *conversation
	logger          *slog.Logger
}

// NewRunController wires the runtime and durable storage at the application boundary.
func NewRunController(ctx context.Context, host agentruntime.RuntimeHost, stateDir string, logger *slog.Logger, scanners ...repositoryobserver.Scanner) *RunController {
	ctx, cancel := context.WithCancel(ctx)
	if logger == nil {
		logger = slog.Default()
	}
	var scanner repositoryobserver.Scanner
	if len(scanners) > 0 {
		scanner = scanners[0]
	}
	conversation := newConversation(ctx, stateDir, logger)
	conversation.repositoryScanner = scanner
	return &RunController{
		ctx: ctx, cancel: cancel, host: host, stateDir: stateDir,
		conversation: conversation,
		checkpoints:  checkpointStore{StateDir: stateDir}, logger: logger,
	}
}

// Configure accepts identical resubmissions and atomically rejects changed bindings.
func (c *RunController) Configure(config configmodel.RunConfig) error {
	return c.configure(config, false)
}

func (c *RunController) configure(config configmodel.RunConfig, recovering bool) error {
	config = config.Clone()
	if err := config.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrLaunchConflict
	}
	if c.checkpointBlocked && !recovering {
		return ErrRecoveryBlocked
	}
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

func (c *RunController) Status() runmodel.RunStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	status := runmodel.RunStatus{
		RecoveryBlocked:    c.recoveryBlocked,
		Configured:         c.runConfig != nil,
		ExecutionAvailable: c.host != nil && !c.closed && !c.recoveryBlocked,
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

type sessionStartMode uint8

const (
	createSession sessionStartMode = iota
	restoreSession
)

// Launch reserves a single attempt before startup I/O. Failed attempts are
// never silently retried; startup belongs to the controller context.
func (c *RunController) Launch() error {
	c.mu.Lock()
	if c.checkpointBlocked {
		c.mu.Unlock()
		return ErrRecoveryBlocked
	}
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
	c.launchState = runmodel.StateStarting
	config := *c.runConfig
	c.startupWorkers.Add(1)
	c.mu.Unlock()
	defer c.startupWorkers.Done()
	return c.startReservedSession(createSession, config)
}

func (c *RunController) startReservedSession(mode sessionStartMode, config configmodel.RunConfig) error {
	startedAt := time.Now()
	launchAttrs := append(runLogAttrs(config), "task_session_id", config.TaskSessionID, "restore_session", mode == restoreSession)
	c.logger.Info("runtime launch started", launchAttrs...)
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	var session agentruntime.RuntimeSession
	var err error
	if mode == restoreSession {
		session, err = c.host.Restore(ctx, config.ForRuntime())
	} else {
		session, err = c.host.Launch(ctx, config.ForRuntime())
	}
	if err != nil {
		return c.recordStartupFailure(mode, launchAttrs, startedAt, err)
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		_ = session.Close()
		return nil
	}
	c.session = session
	c.conversation.bind(config, session)
	conversation := c.conversation
	c.mu.Unlock()
	err = c.initializeConversation(ctx, mode, config, conversation)
	if err != nil {
		_ = session.Close()
		conversation.setExecutionState(runmodel.StateInterrupted)
		c.mu.Lock()
		c.launchState, c.recoveryBlocked = runmodel.StateInterrupted, true
		c.mu.Unlock()
		return err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		_ = session.Close()
		return nil
	}
	c.launchState = runmodel.StateReady
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
	// Checkpoint reads/replay may precede runtime startup admission. Join them too.
	c.checkpointMu.Lock()
	defer c.checkpointMu.Unlock()
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

func runLogAttrs(config configmodel.RunConfig) []any {
	return []any{"organization_id", config.OrganizationID, "workspace_id", config.WorkspaceID, "task_id", config.TaskID, "run_id", config.RunID, "runtime", config.Runtime}
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

func (c *RunController) recordStartupFailure(mode sessionStartMode, attrs []any, startedAt time.Time, err error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.launchState = runmodel.StateFailed
	if mode == restoreSession {
		c.launchState, c.recoveryBlocked = runmodel.StateInterrupted, true
	}
	c.logger.Error("runtime launch failed", append(attrs, "error_type", fmt.Sprintf("%T", err), "duration_ms", time.Since(startedAt).Milliseconds())...)
	return err
}

func (c *RunController) initializeConversation(ctx context.Context, mode sessionStartMode, config configmodel.RunConfig, conversation *conversation) error {
	if config.TaskSessionID != "" && c.checkpoints.Save(config) != nil {
		return ErrJournal
	}
	if mode == restoreSession {
		return conversation.ReplayAndReconcile(ctx)
	}
	return nil
}

func (c *RunController) RespondQuestion(response interaction.QuestionResponse) error {
	return c.currentConversation().RespondQuestion(response)
}
