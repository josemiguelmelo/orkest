package run

import (
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
)

// startNextUnderTurnAndEventLocks requires turnMu and eventMu. It persists delivery before launching
// the next turn and records the run outcome when a task-session queue drains.
func (c *conversation) startNextUnderTurnAndEventLocks() error {
	c.mu.RLock()
	unavailable := c.closed || c.state.paused || c.state.activeCommandID != ""
	var next interaction.Command
	hasNext := len(c.state.queue) != 0
	if hasNext {
		next = c.state.queue[0]
	}
	usesTaskSession, runFailed := c.state.usesTaskSession, c.state.runFailed
	c.mu.RUnlock()
	if unavailable {
		return nil
	}
	if !hasNext {
		if usesTaskSession {
			eventType := interaction.EventRunCompleted
			if runFailed {
				eventType = interaction.EventRunFailed
			}
			return c.commitEventUnderEventLock(interaction.Event{Type: eventType})
		}
		return nil
	}
	if usesTaskSession {
		if err := c.commitEventUnderEventLock(interaction.Event{Type: interaction.EventPromptStarted, CommandID: next.ID}); err != nil {
			return err
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// stop and work admission share mu, so close cannot miss a new worker.
	if c.closed || c.ctx.Err() != nil {
		return interaction.ErrConversation
	}
	if !usesTaskSession {
		c.state.beginCommand(next.ID)
	}
	c.executionState = StateRunning
	c.work.Add(1)
	go c.runPrompt(c.session, next)
	return nil
}

func (c *conversation) ResumeQueue() error { return c.changeQueue(interaction.EventQueueResumed) }

func (c *conversation) DiscardQueue() error { return c.changeQueue(interaction.EventQueueDiscarded) }

func (c *conversation) changeQueue(eventType interaction.EventType) error {
	c.turnMu.Lock()
	defer c.turnMu.Unlock()
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.mu.RLock()
	unavailable := c.closed || c.session == nil || !c.state.paused || c.state.activeCommandID != "" ||
		(c.ctx.Err() != nil && eventType != interaction.EventQueueDiscarded)
	c.mu.RUnlock()
	if unavailable {
		return interaction.ErrConversation
	}
	if err := c.commitEventUnderEventLock(interaction.Event{Type: eventType}); err != nil {
		return err
	}
	c.setExecutionState(StateReady)
	return c.startNextUnderTurnAndEventLocks()
}
