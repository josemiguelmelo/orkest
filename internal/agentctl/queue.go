package agentctl

import (
	"crypto/sha256"
	"strings"
)

// startNextLocked runs with turnMu, eventMu and mu held. Completion and admission
// share these locks, so draining the queue atomically fences the finished run.
func (c *ConversationService) startNextLocked(runtime RuntimeConversation) error {
	if c.closed || c.paused || c.activeCommand != "" {
		return nil
	}
	if len(c.queue) == 0 {
		if c.config.TaskSessionID != "" {
			event := Event{Type: EventRunCompleted}
			if c.runFailed {
				event.Type = EventRunFailed
			}
			if err := c.recordLocked(event); err != nil {
				return err
			}
			c.runEnded = true
		}
		return nil
	}
	next := c.queue[0]
	if c.config.TaskSessionID != "" {
		if err := c.recordLocked(Event{Type: EventPromptStarted, CommandID: next.commandID}); err != nil {
			return err
		}
	}
	c.queue = c.queue[1:]
	c.activeCommand, c.state = next.commandID, StateRunning
	c.commandStates[next.commandID] = "running"
	c.work.Add(1)
	go c.runPrompt(runtime, next.commandID, next.text)
	return nil
}

func (c *ConversationService) replayEvent(event Event) {
	switch event.Type {
	case EventPromptAccepted:
		c.commands[event.CommandID] = sha256.Sum256([]byte(event.Text))
		c.commandStates[event.CommandID] = "queued"
		c.queue = append(c.queue, queuedPrompt{event.CommandID, event.Text})
	case EventPromptContinued:
		c.commandStates[event.CommandID] = "queued"
		c.queue = append([]queuedPrompt{{event.CommandID, event.Text}}, c.queue...)
	case EventPromptStarted:
		c.commandStates[event.CommandID] = "running"
		c.removeQueued(event.CommandID)
	case EventPromptCompleted, EventPromptFailed, EventPromptInterrupted:
		c.commandStates[event.CommandID] = "completed"
		if event.Type != EventPromptCompleted {
			c.paused, c.runFailed = true, true
			c.commandStates[event.CommandID] = "interrupted"
		}
		if event.StopReason == "cancelled" {
			c.paused, c.runFailed = true, true
		}
		c.removeQueued(event.CommandID)
	case EventQueuePaused, EventCancelRequested:
		c.paused, c.runFailed = true, true
	case EventQueueResumed:
		c.paused = false
		if len(c.queue) > 0 {
			c.runFailed = false
		}
	case EventQueueDiscarded:
		c.paused = false
		for _, prompt := range c.queue {
			c.commandStates[prompt.commandID] = "discarded"
		}
		c.queue = nil
	case EventRunCompleted, EventRunFailed:
		c.runEnded, c.paused = true, false
	}
	// Permission IDs must never repeat after restoring the same runtime session.
	if strings.HasPrefix(event.RequestID, "permission-") {
		c.permissionSequence = max(c.permissionSequence, event.Sequence)
	}
}

func (c *ConversationService) removeQueued(commandID string) {
	for i, prompt := range c.queue {
		if prompt.commandID == commandID {
			c.queue = append(c.queue[:i], c.queue[i+1:]...)
			return
		}
	}
}

func (c *ConversationService) restoreConversation() error {
	c.turnMu.Lock()
	defer c.turnMu.Unlock()
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.restore = true
	if err := c.openJournalLocked(); err != nil {
		return err
	}
	if c.runEnded {
		return nil
	}
	history, _ := c.journal.Replay(0)
	// Full accepted text is needed only for the interrupted command; the journal
	// replay window may have expired, in which case recovery remains conservative.
	for commandID, state := range c.commandStates {
		if state == "running" {
			var original string
			for _, event := range history {
				if (event.Type == EventPromptAccepted || event.Type == EventPromptContinued) && event.CommandID == commandID {
					original = event.Text
				}
			}
			recovery, supported := c.process.(RuntimeRecovery)
			if !c.paused && supported && recovery.CanContinue(commandID, original) {
				text := "Continue the interrupted response to the previous user message from the restored conversation. No tool execution was recorded for that turn. Check the current task state before taking any action."
				if err := c.recordLocked(Event{Type: EventPromptContinued, CommandID: commandID, Text: text}); err != nil {
					return err
				}
				c.commandStates[commandID] = "queued"
				c.queue = append([]queuedPrompt{{commandID, text}}, c.queue...)
				continue
			}
			c.paused, c.runFailed = true, true
			c.commandStates[commandID] = "interrupted"
			if err := c.recordLocked(Event{Type: EventPromptInterrupted, CommandID: commandID, Error: "runtime delivery or unfinished effects require reconciliation; original prompt was not resent"}); err != nil {
				return err
			}
		}
	}
	if err := c.recordLocked(Event{Type: EventSessionRestored}); err != nil {
		return err
	}
	if c.paused {
		c.state = StateInterrupted
		return c.recordLocked(Event{Type: EventQueuePaused})
	}
	if len(c.queue) != 0 || len(c.commands) != 0 {
		return c.startNextLocked(c.process.(RuntimeConversation))
	}
	return nil
}

func (c *RunController) Command(commandID string) (CommandStatus, error) {
	v := c.conversationSnapshot()
	v.mu.RLock()
	defer v.mu.RUnlock()
	state, ok := v.commandStates[commandID]
	if !ok {
		return CommandStatus{}, ErrCommand
	}
	return CommandStatus{CommandID: commandID, Status: state}, nil
}

func (c *RunController) Queue(discard bool) error {
	v := c.conversationSnapshot()
	v.turnMu.Lock()
	defer v.turnMu.Unlock()
	v.eventMu.Lock()
	defer v.eventMu.Unlock()
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed || v.process == nil || !v.paused || v.activeCommand != "" || (v.ctx.Err() != nil && !discard) {
		return ErrConversation
	}
	runtime, ok := v.process.(RuntimeConversation)
	if !ok {
		return ErrConversation
	}
	event := Event{Type: EventQueueResumed}
	if discard {
		event.Type = EventQueueDiscarded
	}
	if err := v.recordLocked(event); err != nil {
		return err
	}
	if discard {
		for _, prompt := range v.queue {
			v.commandStates[prompt.commandID] = "discarded"
		}
		v.queue = nil
	}
	if !discard && len(v.queue) > 0 {
		v.runFailed = false
	}
	v.paused, v.cancelled, v.state = false, false, StateReady
	return v.startNextLocked(runtime)
}
