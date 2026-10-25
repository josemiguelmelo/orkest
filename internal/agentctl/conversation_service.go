package agentctl

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"
)

type pendingPermission struct {
	permission Permission
	commandID  string
	response   chan string
	answered   bool
}

// ConversationService owns prompt execution, pending permissions, and activity
// for one bound runtime session. It does not launch or close the runtime process.
type ConversationService struct {
	// turnMu serializes acceptance, completion, and cancellation delivery.
	// Acquire turnMu before mu when both are needed.
	turnMu sync.Mutex
	// mu protects conversation state, the journal, subscriptions, and work admission.
	mu                 sync.RWMutex
	ctx                context.Context
	cancel             context.CancelFunc
	config             *RunConfig
	process            Process
	state              State
	closed             bool
	work               sync.WaitGroup
	stateDir           string
	journal            *Journal
	subscriptions      Subscriptions
	commands           map[string][sha256.Size]byte
	pending            map[string]*pendingPermission
	activeCommand      string
	permissionSequence uint64
}

func newConversationService(ctx context.Context, stateDir string) *ConversationService {
	ctx, cancel := context.WithCancel(ctx)
	return &ConversationService{ctx: ctx, cancel: cancel, stateDir: stateDir}
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
	c.mu.Lock()
	defer c.mu.Unlock()
	c.subscriptions.Close()
	if c.journal != nil {
		return c.journal.Close()
	}
	return nil
}

// openJournalLocked refuses to replace an earlier process's history or ownership.
func (c *ConversationService) openJournalLocked() error {
	if c.journal != nil {
		return nil
	}
	journal, err := openJournal(c.stateDir, *c.config)
	if err != nil {
		return err
	}
	c.journal = journal
	c.commands = make(map[string][sha256.Size]byte)
	c.pending = make(map[string]*pendingPermission)
	return nil
}

// recordLocked serializes durable append and publication with state transitions.
// The caller holds mu. Subscribers never observe an event before it is synced.
func (c *ConversationService) recordLocked(event Event) error {
	config := c.config
	event.OrganizationID, event.WorkspaceID, event.TaskID, event.RunID = config.OrganizationID, config.WorkspaceID, config.TaskID, config.RunID
	event.SessionID = c.process.SessionID()
	event.Sequence = c.journal.sequence + 1
	event.At = time.Now().UTC()
	if err := c.journal.Append(event); err != nil {
		c.state = StateInterrupted
		return err
	}
	c.subscriptions.Publish(event)
	return nil
}

// Prompt accepts an idempotent command before invoking the provider. It is owned
// by the conversation context and continues when its requesting connection disappears.
func (c *ConversationService) Prompt(commandID, text string) error {
	if strings.TrimSpace(commandID) == "" || len(commandID) > 200 || len(text) > MaxPromptBytes || strings.TrimSpace(text) == "" {
		return ErrCommand
	}
	c.turnMu.Lock()
	defer c.turnMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.process == nil || c.state == StateFailed {
		return ErrConversation
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
		return err
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
	if c.activeCommand != "" {
		return ErrConversationBusy
	}
	if err := c.recordLocked(Event{Type: "prompt_accepted", CommandID: commandID, Text: text}); err != nil {
		return err
	}
	c.commands[commandID] = sha256.Sum256([]byte(text))
	c.activeCommand = commandID
	c.state = StateRunning
	c.work.Add(1)
	go c.runPrompt(runtime, commandID, text)
	return nil
}

func (c *ConversationService) runPrompt(runtime RuntimeConversation, commandID, text string) {
	defer c.work.Done()
	reason, err := runtime.Prompt(c.ctx, text, Observer{
		Update: func(update Update) error {
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.activeCommand != commandID {
				return ErrConversation
			}
			return c.recordLocked(Event{Type: "update", CommandID: commandID, Update: &update})
		},
		Permission: func(ctx context.Context, p Permission) (string, error) { return c.waitPermission(ctx, commandID, p) },
	})
	c.turnMu.Lock()
	defer c.turnMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	event := Event{Type: "prompt_completed", CommandID: commandID, StopReason: reason}
	if err != nil {
		event.Type = "prompt_failed"
		event.Error = "provider prompt failed; outcome may be uncertain"
		c.state = StateInterrupted
	} else if c.state != StateInterrupted {
		c.state = StateReady
	}
	for id, p := range c.pending {
		if p.commandID == commandID {
			delete(c.pending, id)
		}
	}
	c.activeCommand = ""
	_ = c.recordLocked(event)
}

func (c *ConversationService) waitPermission(ctx context.Context, commandID string, p Permission) (string, error) {
	c.mu.Lock()
	if c.closed || c.state == StateInterrupted || c.activeCommand != commandID || ctx.Err() != nil {
		c.mu.Unlock()
		return "", ErrConversation
	}
	c.permissionSequence++
	id := fmt.Sprintf("permission-%d", c.permissionSequence)
	pending := &pendingPermission{permission: p, commandID: commandID, response: make(chan string, 1)}
	if err := c.recordLocked(Event{Type: "permission_requested", CommandID: commandID, RequestID: id, Permission: &p}); err != nil {
		c.mu.Unlock()
		return "", err
	}
	c.pending[id] = pending
	c.state = StateWaitingPermission
	c.mu.Unlock()
	defer func() {
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
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.config == nil || c.process == nil {
		return ErrPermission
	}
	config := c.config
	if response.TaskID != config.TaskID || response.RunID != config.RunID || response.SessionID != c.process.SessionID() {
		return ErrPermission
	}
	pending, ok := c.pending[response.RequestID]
	if !ok || pending.answered {
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
	if err := c.recordLocked(Event{Type: "permission_response", CommandID: pending.commandID, RequestID: response.RequestID, Text: response.OptionID}); err != nil {
		return err
	}
	pending.answered = true
	pending.response <- response.OptionID
	return nil
}

func (c *ConversationService) CancelPrompt(commandID string) error {
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
		c.state = StateInterrupted
		c.mu.Unlock()
		return ErrCancellation
	}
	return nil
}

func (c *ConversationService) prepareCancellation(commandID string) (RuntimeConversation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.process == nil || commandID == "" || c.activeCommand != commandID {
		return nil, ErrCommand
	}
	runtime, ok := c.process.(RuntimeConversation)
	if !ok {
		return nil, ErrConversation
	}
	if err := c.recordLocked(Event{Type: "cancel_requested", CommandID: commandID}); err != nil {
		return nil, err
	}
	return runtime, nil
}

// Subscribe atomically snapshots replay and starts live delivery. Slow subscribers
// are detached, leaving execution and the private journal unaffected.
func (c *ConversationService) Subscribe(after uint64) ([]Event, <-chan Event, func(), error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, nil, nil, ErrConversation
	}
	events, err := c.journal.Replay(after)
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
