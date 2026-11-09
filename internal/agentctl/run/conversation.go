package run

import (
	"context"
	"log/slog"
	"sync"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	"github.com/josemiguelmelo/orkest/internal/agentctl/repositoryobserver"
	runmodel "github.com/josemiguelmelo/orkest/internal/agentctl/run/model"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

type conversationSnapshot struct {
	State           runmodel.ExecutionState
	ActiveCommandID string
	QueueDepth      int
	QueuePaused     bool
	RunEnded        bool
	Cancelled       bool
	RecoveryAllowed bool
}

// conversation owns prompt execution, pending permissions, and activity
// for one bound runtime session. It does not launch or close the runtime session.
type conversation struct {
	repositoryScanner               repositoryobserver.Scanner
	repositoryObservationState      *repositoryobserver.State
	repositoryObservationDiagnostic string
	repositoryObservationStore      *repositoryObservationStore
	// mu protects durable state, transient execution state, and work admission.
	// Status and stop need only mu.
	mu    sync.RWMutex
	state conversationState

	// turnMu serializes acceptance, completion, and cancellation delivery.
	// Lock order: turnMu, eventMu, mu. Never acquire in reverse order.
	turnMu sync.Mutex
	// eventMu orders commits and replay/live subscription handoff. It remains
	// held while mu is released for storage I/O.
	eventMu sync.Mutex

	// These values belong to the bound runtime and are never replayed. Runtime
	// callbacks update progress; journal events update state. Persistence errors
	// fence execution even when no interruption event could be written.
	config         *configmodel.RunConfig
	session        agentruntime.RuntimeSession
	executionState runmodel.ExecutionState
	pending        map[string]*pendingPermission
	questions      map[string]*pendingQuestion

	// Shutdown cancels accepted workers and prevents further work admission.
	ctx    context.Context
	cancel context.CancelFunc
	closed bool
	work   sync.WaitGroup

	// Journal I/O and live delivery share event ordering, not the state lock.
	stateDir              string
	logger                *slog.Logger
	journal               *Journal
	subscriptions         eventSubscriptions
	replayExistingJournal bool
}

func newConversation(ctx context.Context, stateDir string, logger *slog.Logger) *conversation {
	ctx, cancel := context.WithCancel(ctx)
	if logger == nil {
		logger = slog.Default()
	}
	return &conversation{ctx: ctx, cancel: cancel, stateDir: stateDir, logger: logger}
}

// bind attaches the single successful launch before the controller exposes it.
// RunController serializes binding with launch and shutdown; no reverse dependency
// on the controller is needed by conversation operations.
func (c *conversation) bind(config configmodel.RunConfig, session agentruntime.RuntimeSession) {
	c.mu.Lock()
	defer c.mu.Unlock()
	config = config.Clone()
	c.config, c.session, c.executionState = &config, session, runmodel.StateReady
	c.state.usesTaskSession = config.TaskSessionID != ""
}

func (c *conversation) Snapshot() conversationSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return conversationSnapshot{
		State:           c.executionState,
		ActiveCommandID: c.state.activeCommandID,
		QueueDepth:      len(c.state.queue),
		QueuePaused:     c.state.paused,
		RunEnded:        c.state.runEnded,
		Cancelled:       c.state.cancelled,
		RecoveryAllowed: c.state.recoveryAllowed,
	}
}

// markRecoveredRunEnded reflects a terminal journal already checked by the
// controller during checkpoint recovery; it must not append another event.
func (c *conversation) markRecoveredRunEnded() {
	c.mu.Lock()
	c.state.runEnded = true
	c.executionState = runmodel.StateExited
	c.mu.Unlock()
}

// stop prevents admission before the controller begins session shutdown.
func (c *conversation) stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.cancel()
}

// close waits for accepted prompts before closing their journal and subscriptions.
// The controller stops the runtime first so a blocked provider can finish.
func (c *conversation) close() error {
	c.stop()
	c.work.Wait()
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.mu.Lock()
	c.subscriptions.Close()
	journal := c.journal
	c.mu.Unlock()
	if journal != nil {
		return journal.Close()
	}
	return nil
}

// interrupt fences work and wakes permissions when persistence or delivery fails.
func (c *conversation) interrupt() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.executionState = runmodel.StateInterrupted
	c.cancel()
}

func (c *conversation) setExecutionState(state runmodel.ExecutionState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.executionState = state
}

func (c *conversation) logAttrs(commandID string) []any {
	attrs := []any{"command_id", commandID}
	c.mu.RLock()
	config := c.config
	session := c.session
	c.mu.RUnlock()
	if config != nil {
		attrs = append(attrs, runLogAttrs(*config)...)
	}
	if session != nil {
		attrs = append(attrs, "session_id", session.SessionID())
	}
	return attrs
}
