package agentctl

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrConversation = errors.New("conversation unavailable, busy, or interrupted")
	ErrCommand      = errors.New("invalid or conflicting command identity")
	ErrPermission   = errors.New("permission identity or option does not match a pending request")
)

// Event is sensitive task activity, persisted before subscribers are notified.
type Event struct {
	At             time.Time   `json:"at"`
	Sequence       uint64      `json:"sequence"`
	Type           string      `json:"type"`
	OrganizationID string      `json:"organizationId"`
	WorkspaceID    string      `json:"workspaceId"`
	TaskID         string      `json:"taskId"`
	RunID          string      `json:"runId"`
	SessionID      string      `json:"sessionId"`
	CommandID      string      `json:"commandId,omitempty"`
	RequestID      string      `json:"requestId,omitempty"`
	Text           string      `json:"text,omitempty"`
	Update         *Update     `json:"update,omitempty"`
	Permission     *Permission `json:"permission,omitempty"`
	StopReason     string      `json:"stopReason,omitempty"`
	Error          string      `json:"error,omitempty"`
}

type PermissionResponse struct {
	TaskID    string `json:"taskId"`
	RunID     string `json:"runId"`
	SessionID string `json:"sessionId"`
	RequestID string `json:"requestId"`
	OptionID  string `json:"optionId"`
}

type pendingPermission struct {
	permission Permission
	commandID  string
	response   chan string
	answered   bool
}

type conversationState struct {
	journal            *os.File
	journalFailed      bool
	events             []Event
	commands           map[string]string
	subscribers        map[chan Event]struct{}
	pending            map[string]*pendingPermission
	activeCommand      string
	permissionSequence uint64
}

// openJournalLocked refuses to replace an earlier process's history or ownership.
func (s *Service) openJournalLocked() error {
	if s.conversation.journal != nil {
		return nil
	}
	if s.stateDir == "" || !filepath.IsAbs(s.stateDir) {
		return errors.New("absolute state directory required for conversation journal")
	}
	config := s.configuration
	scope, _ := json.Marshal([]string{config.OrganizationID, config.WorkspaceID, config.TaskID, config.RunID})
	dir := filepath.Join(s.stateDir, "conversations")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errors.New("create conversation storage failed")
	}
	file, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("%x.jsonl", sha256.Sum256(scope))), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.New("conversation history exists or is unavailable; reconciliation required")
	}
	s.conversation.journal = file
	s.conversation.commands = make(map[string]string)
	s.conversation.pending = make(map[string]*pendingPermission)
	return nil
}

func (s *Service) recordLocked(event Event) error {
	c := &s.conversation
	if c.journalFailed {
		s.state = "interrupted"
		return errors.New("conversation journal unavailable")
	}
	config := s.configuration
	event.OrganizationID, event.WorkspaceID, event.TaskID, event.RunID = config.OrganizationID, config.WorkspaceID, config.TaskID, config.RunID
	event.SessionID = s.process.SessionID()
	event.Sequence = uint64(len(c.events)) + 1
	event.At = time.Now().UTC()
	data, err := json.Marshal(event)
	if err == nil {
		_, err = c.journal.Write(append(data, '\n'))
	}
	if err == nil {
		err = c.journal.Sync()
	}
	if err != nil {
		c.journalFailed = true
		s.state = "interrupted"
		return errors.New("persist conversation event failed")
	}
	c.events = append(c.events, event)
	for subscriber := range c.subscribers {
		select {
		case subscriber <- event:
		default:
			close(subscriber)
			delete(c.subscribers, subscriber)
		}
	}
	return nil
}

// Prompt accepts an idempotent command before invoking the provider. It is owned
// by the service context and continues when its requesting connection disappears.
func (s *Service) Prompt(commandID, text string) error {
	if strings.TrimSpace(commandID) == "" || len(commandID) > 200 || strings.TrimSpace(text) == "" {
		return ErrCommand
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.process == nil || s.state == "failed" || s.state == "interrupted" {
		return ErrConversation
	}
	select {
	case <-s.process.Done():
		return ErrConversation
	default:
	}
	runtime, ok := s.process.(Conversation)
	if !ok {
		return ErrConversation
	}
	if err := s.openJournalLocked(); err != nil {
		return err
	}
	c := &s.conversation
	if previous, exists := c.commands[commandID]; exists {
		if previous == text {
			return nil
		}
		return ErrCommand
	}
	if c.activeCommand != "" {
		return ErrConversation
	}
	if err := s.recordLocked(Event{Type: "prompt_accepted", CommandID: commandID, Text: text}); err != nil {
		return err
	}
	c.commands[commandID] = text
	c.activeCommand = commandID
	s.state = "running"
	s.launches.Add(1)
	go s.runPrompt(runtime, commandID, text)
	return nil
}

func (s *Service) runPrompt(runtime Conversation, commandID, text string) {
	defer s.launches.Done()
	reason, err := runtime.Prompt(s.ctx, text, Observer{
		Update: func(update Update) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.conversation.activeCommand != commandID {
				return ErrConversation
			}
			return s.recordLocked(Event{Type: "update", CommandID: commandID, Update: &update})
		},
		Permission: func(ctx context.Context, p Permission) (string, error) { return s.waitPermission(ctx, commandID, p) },
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	event := Event{Type: "prompt_completed", CommandID: commandID, StopReason: reason}
	if err != nil {
		event.Type = "prompt_failed"
		event.Error = "provider prompt failed; outcome may be uncertain"
		s.state = "interrupted"
	} else {
		s.state = "ready"
	}
	for id, p := range s.conversation.pending {
		if p.commandID == commandID {
			delete(s.conversation.pending, id)
		}
	}
	s.conversation.activeCommand = ""
	_ = s.recordLocked(event)
}

func (s *Service) waitPermission(ctx context.Context, commandID string, p Permission) (string, error) {
	s.mu.Lock()
	if s.closed || s.conversation.activeCommand != commandID || ctx.Err() != nil {
		s.mu.Unlock()
		return "", ErrConversation
	}
	s.conversation.permissionSequence++
	id := fmt.Sprintf("permission-%d", s.conversation.permissionSequence)
	pending := &pendingPermission{permission: p, commandID: commandID, response: make(chan string, 1)}
	if err := s.recordLocked(Event{Type: "permission_requested", CommandID: commandID, RequestID: id, Permission: &p}); err != nil {
		s.mu.Unlock()
		return "", err
	}
	s.conversation.pending[id] = pending
	s.state = "waiting_for_permission"
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conversation.pending, id)
		if s.state == "waiting_for_permission" && len(s.conversation.pending) == 0 {
			s.state = "running"
		}
		s.mu.Unlock()
	}()
	select {
	case option := <-pending.response:
		return option, nil
	case <-ctx.Done():
		return "", ctx.Err()
	case <-s.ctx.Done():
		return "", s.ctx.Err()
	}
}

func (s *Service) RespondPermission(response PermissionResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.configuration == nil || s.process == nil {
		return ErrPermission
	}
	config := s.configuration
	if response.TaskID != config.TaskID || response.RunID != config.RunID || response.SessionID != s.process.SessionID() {
		return ErrPermission
	}
	pending, ok := s.conversation.pending[response.RequestID]
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
	if err := s.recordLocked(Event{Type: "permission_response", CommandID: pending.commandID, RequestID: response.RequestID, Text: response.OptionID}); err != nil {
		return err
	}
	pending.answered = true
	pending.response <- response.OptionID
	return nil
}

func (s *Service) CancelPrompt(commandID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.process == nil || s.conversation.activeCommand == "" || s.conversation.activeCommand != commandID {
		return ErrCommand
	}
	runtime, ok := s.process.(Conversation)
	if !ok {
		return ErrConversation
	}
	if err := s.recordLocked(Event{Type: "cancel_requested", CommandID: commandID}); err != nil {
		return err
	}
	// Keep completion/new prompts serialized with notification delivery so a late
	// cancellation cannot cancel the next turn in the same ACP session.
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	if err := runtime.Cancel(ctx); err != nil {
		s.state = "interrupted"
		return errors.New("provider cancellation delivery failed")
	}
	return nil
}

// Subscribe atomically snapshots replay and starts live delivery. Slow subscribers
// are detached, leaving execution and the private journal unaffected.
func (s *Service) Subscribe(after uint64) ([]Event, <-chan Event, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, nil, nil, ErrConversation
	}
	if after > uint64(len(s.conversation.events)) {
		return nil, nil, nil, ErrCommand
	}
	events := append([]Event(nil), s.conversation.events[after:]...)
	channel := make(chan Event, 64)
	if s.conversation.subscribers == nil {
		s.conversation.subscribers = make(map[chan Event]struct{})
	}
	s.conversation.subscribers[channel] = struct{}{}
	stop := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := s.conversation.subscribers[channel]; ok {
			delete(s.conversation.subscribers, channel)
			close(channel)
		}
	}
	return events, channel, stop, nil
}
