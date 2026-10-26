package agentctl

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type pendingPermission struct {
	permission Permission
	ctx        context.Context
	commandID  string
	response   chan string
	answered   bool
}

// ConversationService owns prompt execution, pending permissions, and activity
// for one bound runtime session. It does not launch or close the runtime process.
type ConversationService struct {
	// turnMu serializes acceptance, completion, and cancellation delivery.
	// Lock order: turnMu, eventMu, mu. Never acquire in reverse order.
	turnMu sync.Mutex
	// eventMu orders durable events and their state transitions, including replay
	// subscription handoff. It remains held while mu is released for storage I/O.
	eventMu sync.Mutex
	// mu protects in-memory state and work admission. Status and stop need only mu.
	mu                 sync.RWMutex
	ctx                context.Context
	cancel             context.CancelFunc
	config             *RunConfig
	process            Process
	state              State
	closed             bool
	work               sync.WaitGroup
	stateDir           string
	logger             *slog.Logger
	journal            *Journal
	subscriptions      EventSubscriptions
	commands           map[string][sha256.Size]byte
	pending            map[string]*pendingPermission
	activeCommand      string
	permissionSequence uint64
	queue              []queuedPrompt
	commandStates      map[string]string
	paused             bool
	cancelled          bool
	runEnded           bool
	runFailed          bool
	restore            bool
}

func newConversationService(ctx context.Context, stateDir string, loggers ...*slog.Logger) *ConversationService {
	ctx, cancel := context.WithCancel(ctx)
	logger := slog.Default()
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	return &ConversationService{ctx: ctx, cancel: cancel, stateDir: stateDir, logger: logger}
}

// bind attaches the single successful launch before the controller exposes it.
// RunController serializes binding with launch and shutdown; no reverse dependency
// on the controller is needed by conversation operations.
func (c *ConversationService) bind(config RunConfig, process Process) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config, c.process, c.state = &config, process, StateReady
}

func (c *ConversationService) stateSnapshot() State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// stop prevents admission before the controller begins process shutdown.
func (c *ConversationService) stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	c.cancel()
}

// close waits for accepted prompts before closing their journal and subscriptions.
// The controller stops the runtime first so a blocked provider can finish.
func (c *ConversationService) close() error {
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

// openJournalLocked requires eventMu and mu. It releases mu for filesystem I/O
// and refuses to replace an earlier process's history or ownership.
func (c *ConversationService) openJournalLocked() error {
	if c.journal != nil {
		return nil
	}
	config := *c.config
	c.mu.Unlock()
	journal, history, err := loadJournal(c.stateDir, config, c.restore)
	c.mu.Lock()
	if err != nil {
		return err
	}
	c.journal = journal
	c.commands = make(map[string][sha256.Size]byte)
	c.pending = make(map[string]*pendingPermission)
	c.commandStates = make(map[string]string)
	for _, event := range history {
		c.replayEvent(event)
	}
	return nil
}

// interruptLocked fences new work and wakes every pending permission without
// selecting an option. Cancellation does not prove the runtime has stopped.
func (c *ConversationService) interruptLocked() {
	c.state = StateInterrupted
	c.cancel()
}

// recordLocked requires eventMu and mu, and returns with both still held.
// Storage I/O releases mu so status and shutdown admission remain responsive.
// eventMu prevents replay/live gaps and reordering while a commit is in flight.
func (c *ConversationService) recordLocked(event Event) error {
	config := c.config
	event.OrganizationID, event.WorkspaceID, event.TaskID, event.RunID = config.OrganizationID, config.WorkspaceID, config.TaskID, config.RunID
	event.SessionID = c.process.SessionID()
	event.TaskSessionID = config.TaskSessionID
	c.mu.Unlock()
	committed, err := c.journal.Append(event)
	c.mu.Lock()
	if err != nil {
		c.interruptLocked()
		return err
	}
	c.subscriptions.Publish(committed)
	return nil
}

// Prompt accepts an idempotent command before invoking the provider. It is owned
// by the conversation context and continues when its requesting connection disappears.
func (c *ConversationService) Prompt(commandID, text string) error {
	logAttrs := c.logAttrs(commandID)
	c.logger.Info("prompt received", logAttrs...)
	if strings.TrimSpace(commandID) == "" || len(commandID) > 200 || len(text) > MaxPromptBytes || strings.TrimSpace(text) == "" {
		c.logger.Warn("prompt rejected", append(logAttrs, "reason", "invalid_command")...)
		return ErrCommand
	}
	c.turnMu.Lock()
	defer c.turnMu.Unlock()
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.process == nil || c.state == StateFailed {
		return ErrConversation
	}
	if previous, exists := c.commands[commandID]; exists {
		if previous == sha256.Sum256([]byte(text)) {
			return nil
		}
		return ErrCommand
	}
	if c.state == StateInterrupted {
		return ErrConversationInterrupted
	}
	select {
	case <-c.process.Done():
		return ErrConversation
	default:
	}
	runtime, ok := c.process.(RuntimeConversation)
	if !ok {
		return ErrConversation
	}
	if err := c.openJournalLocked(); err != nil {
		c.logger.Error("prompt journal setup failed", append(logAttrs, "reason", "journal_unavailable")...)
		return err
	}
	if c.closed || c.ctx.Err() != nil {
		return ErrConversation
	}
	if previous, exists := c.commands[commandID]; exists {
		if previous == sha256.Sum256([]byte(text)) {
			return nil
		}
		return ErrCommand
	}
	if len(c.commands) >= MaxCommands {
		return ErrCommandLimit
	}
	if c.runEnded || c.paused {
		return ErrConversationInterrupted
	}
	if c.activeCommand != "" && c.config.TaskSessionID == "" {
		return ErrConversationBusy
	}
	if err := c.recordLocked(promptAcceptedEvent(commandID, text)); err != nil {
		c.logger.Error("prompt acceptance persistence failed", append(logAttrs, "reason", "journal_write_failed")...)
		return err
	}
	if c.closed || c.ctx.Err() != nil {
		c.interruptLocked()
		_ = c.recordLocked(promptResultEvent(commandID, "", context.Canceled))
		return ErrConversation
	}
	c.commands[commandID] = sha256.Sum256([]byte(text))
	c.commandStates[commandID] = "queued"
	c.queue = append(c.queue, queuedPrompt{commandID, text})
	if c.activeCommand == "" {
		return c.startNextLocked(runtime)
	}
	return nil
}

func (c *ConversationService) runPrompt(runtime RuntimeConversation, commandID, text string) {
	defer c.work.Done()
	startedAt := time.Now()
	logAttrs := c.logAttrs(commandID)
	c.logger.Info("runtime prompt started", logAttrs...)
	observer := Observer{
		Update: func(update Update) error {
			c.eventMu.Lock()
			defer c.eventMu.Unlock()
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.closed || c.state == StateInterrupted || c.activeCommand != commandID {
				return ErrConversation
			}
			return c.recordLocked(updateEvent(commandID, update))
		},
		Permission: func(ctx context.Context, p Permission) (string, error) { return c.waitPermission(ctx, commandID, p) },
	}
	var reason string
	var err error
	if commandRuntime, ok := runtime.(RuntimeCommandConversation); ok && c.config.TaskSessionID != "" {
		reason, err = commandRuntime.PromptCommand(c.ctx, commandID, text, observer)
	} else {
		reason, err = runtime.Prompt(c.ctx, text, observer)
	}
	c.turnMu.Lock()
	defer c.turnMu.Unlock()
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	event := promptResultEvent(commandID, reason, err)
	if err != nil && c.config.TaskSessionID != "" && !c.cancelled && (c.closed || c.ctx.Err() != nil || c.runtimeStopped(runtime)) {
		// A disconnected transport is an infrastructure interruption, not a failed
		// user turn. Keep delivery unresolved for the restoration reconciliation.
		c.activeCommand, c.state = "", StateInterrupted
		c.paused = true
		_ = c.recordLocked(Event{Type: EventRuntimeStopped, CommandID: commandID, Error: "runtime stopped; restoring durable session"})
		return
	}
	if err != nil {
		if c.config.TaskSessionID == "" {
			c.interruptLocked()
		} else {
			c.state = StateInterrupted
		}
	}
	for id, p := range c.pending {
		if p.commandID == commandID {
			delete(c.pending, id)
		}
	}
	c.activeCommand = ""
	c.commandStates[commandID] = "completed"
	if err != nil {
		c.commandStates[commandID] = "failed"
	}
	if recordErr := c.recordLocked(event); recordErr != nil {
		return
	}
	if c.config.TaskSessionID != "" {
		if err != nil || c.cancelled || reason == "cancelled" || c.state == StateInterrupted {
			c.paused, c.runFailed, c.state = true, true, StateInterrupted
			_ = c.recordLocked(Event{Type: EventQueuePaused})
		} else {
			c.state = StateReady
			_ = c.startNextLocked(runtime)
		}
	} else if c.state != StateInterrupted {
		c.state = StateReady
	}
	attrs := append(logAttrs, "duration_ms", time.Since(startedAt).Milliseconds())
	if err != nil {
		c.logger.Warn("runtime prompt finished with failure", append(attrs, "reason", "provider_error")...)
		return
	}
	c.logger.Info("runtime prompt finished", append(attrs, "has_stop_reason", reason != "")...)
}

// registerPermission serializes request persistence with response admission.
func (c *ConversationService) registerPermission(ctx context.Context, commandID string, p Permission) (string, *pendingPermission, error) {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.state == StateInterrupted || c.activeCommand != commandID || ctx.Err() != nil {
		return "", nil, ErrConversation
	}
	c.permissionSequence++
	id := fmt.Sprintf("permission-%d", c.permissionSequence)
	pending := &pendingPermission{permission: p, commandID: commandID, response: make(chan string, 1), ctx: ctx}
	if err := c.recordLocked(permissionRequestedEvent(commandID, id, p)); err != nil {
		return "", nil, err
	}
	if c.closed || c.ctx.Err() != nil || ctx.Err() != nil {
		return "", nil, ErrConversation
	}
	c.pending[id] = pending
	c.state = StateWaitingPermission
	return id, pending, nil
}

func (c *ConversationService) waitPermission(ctx context.Context, commandID string, p Permission) (string, error) {
	id, pending, err := c.registerPermission(ctx, commandID, p)
	if err != nil {
		return "", err
	}
	defer func() {
		c.eventMu.Lock()
		defer c.eventMu.Unlock()
		c.mu.Lock()
		delete(c.pending, id)
		if c.state == StateWaitingPermission && len(c.pending) == 0 {
			c.state = StateRunning
		}
		c.mu.Unlock()
	}()
	select {
	case option := <-pending.response:
		return option, nil
	case <-ctx.Done():
		return "", ctx.Err()
	case <-c.ctx.Done():
		return "", c.ctx.Err()
	}
}

func (c *ConversationService) RespondPermission(response PermissionResponse) error {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.state == StateInterrupted || c.config == nil || c.process == nil {
		return ErrPermission
	}
	config := c.config
	if response.TaskID != config.TaskID || response.RunID != config.RunID || response.SessionID != c.process.SessionID() {
		return ErrPermission
	}
	pending, ok := c.pending[response.RequestID]
	if !ok || pending.answered || pending.ctx.Err() != nil {
		return ErrPermission
	}
	valid := false
	for _, option := range pending.permission.Options {
		if option.ID == response.OptionID {
			valid = true
		}
	}
	if !valid {
		return ErrPermission
	}
	// Delivery intent is not evidence that the provider accepted this answer.
	if err := c.recordLocked(permissionResponseEvent(pending.commandID, response)); err != nil {
		return err
	}
	if c.closed || c.ctx.Err() != nil || pending.ctx.Err() != nil {
		return ErrPermission
	}
	pending.answered = true
	pending.response <- response.OptionID
	return nil
}

func (c *ConversationService) CancelPrompt(commandID string) error {
	c.logger.Info("prompt cancellation requested", c.logAttrs(commandID)...)
	// Serialize delivery with completion and new turns, but allow status, updates,
	// and permission responses to proceed while provider I/O is in flight.
	c.turnMu.Lock()
	defer c.turnMu.Unlock()
	runtime, err := c.prepareCancellation(commandID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()
	if err := runtime.Cancel(ctx); err != nil {
		c.mu.Lock()
		c.interruptLocked()
		c.mu.Unlock()
		return ErrCancellation
	}
	return nil
}

func (c *ConversationService) prepareCancellation(commandID string) (RuntimeConversation, error) {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.process == nil || commandID == "" || c.activeCommand != commandID {
		return nil, ErrCommand
	}
	runtime, ok := c.process.(RuntimeConversation)
	if !ok {
		return nil, ErrConversation
	}
	c.cancelled = true
	if err := c.recordLocked(cancelRequestedEvent(commandID)); err != nil {
		return nil, err
	}
	return runtime, nil
}

// Subscribe atomically snapshots replay and starts live delivery. Slow subscribers
// are detached, leaving execution and the private journal unaffected.
func (c *ConversationService) Subscribe(after uint64) ([]Event, <-chan Event, func(), error) {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, nil, nil, ErrConversation
	}
	journal := c.journal
	c.mu.Unlock()
	events, err := journal.Replay(after)
	c.mu.Lock()
	if c.closed {
		return nil, nil, nil, ErrConversation
	}
	if err != nil {
		return nil, nil, nil, err
	}
	channel := c.subscriptions.Add()
	stop := func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.subscriptions.Remove(channel)
	}
	return events, channel, stop, nil
}

func (c *ConversationService) logAttrs(commandID string) []any {
	attrs := []any{"command_id", commandID}
	c.mu.RLock()
	config := c.config
	process := c.process
	c.mu.RUnlock()
	if config != nil {
		attrs = append(attrs, runLogAttrs(*config)...)
	}
	if process != nil {
		attrs = append(attrs, "session_id", process.SessionID())
	}
	return attrs
}

func (c *ConversationService) runtimeStopped(runtime RuntimeConversation) bool {
	if connection, ok := runtime.(interface{ ConnectionLost() bool }); ok && connection.ConnectionLost() {
		return true
	}
	select {
	case <-c.process.Done():
		return true
	default:
		return false
	}
}
