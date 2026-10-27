package run

import (
	"crypto/sha256"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
)

type commandRecord struct {
	contentHash [sha256.Size]byte
	state       interaction.CommandState
}

// conversationState is the durable state machine. Live commits and restoration
// apply identical transitions; channels, observers, and I/O live in the service.
type conversationState struct {
	usesTaskSession    bool
	commands           map[string]commandRecord
	queue              []interaction.Command
	activeCommandID    string
	permissionSequence uint64
	paused             bool
	cancelled          bool
	runEnded           bool
	runFailed          bool
	recoveryAllowed    bool
}

func (c *conversationState) apply(event interaction.Event) {
	if c.commands == nil {
		c.commands = make(map[string]commandRecord)
	}
	// Activity/permission events preserve recovery eligibility; only lifecycle
	// decisions revoke it. Runtime interruption explicitly grants it below.
	switch event.Type {
	case interaction.EventPromptAccepted, interaction.EventPromptContinued, interaction.EventPromptStarted,
		interaction.EventPromptCompleted, interaction.EventPromptFailed, interaction.EventPromptInterrupted,
		interaction.EventCancelRequested, interaction.EventQueuePaused, interaction.EventQueueResumed, interaction.EventQueueDiscarded,
		interaction.EventRunCompleted, interaction.EventRunFailed, interaction.EventSessionRestored:
		c.recoveryAllowed = false
	}
	switch event.Type {
	case interaction.EventPromptAccepted:
		c.commands[event.CommandID] = commandRecord{contentHash: sha256.Sum256([]byte(event.Text)), state: interaction.CommandQueued}
		c.queue = append(c.queue, interaction.Command{ID: event.CommandID, Text: event.Text})
	case interaction.EventPromptContinued:
		if c.activeCommandID == event.CommandID {
			c.activeCommandID = ""
		}
		c.setCommandState(event.CommandID, interaction.CommandQueued)
		c.queue = append([]interaction.Command{{ID: event.CommandID, Text: event.Text}}, c.queue...)
	case interaction.EventPromptStarted:
		c.beginCommand(event.CommandID)
	case interaction.EventPromptCompleted, interaction.EventPromptFailed, interaction.EventPromptInterrupted:
		commandState := interaction.CommandCompleted
		switch event.Type {
		case interaction.EventPromptFailed:
			commandState = interaction.CommandFailed
		case interaction.EventPromptInterrupted:
			commandState = interaction.CommandInterrupted
		}
		c.setCommandState(event.CommandID, commandState)
		if event.Type != interaction.EventPromptCompleted {
			c.paused, c.runFailed = true, true
		}
		if c.usesTaskSession && event.StopReason == "cancelled" {
			c.paused, c.runFailed = true, true
		}
		c.removeQueued(event.CommandID)
		if c.activeCommandID == event.CommandID {
			c.activeCommandID = ""
		}
	case interaction.EventRuntimeStopped:
		c.recoveryAllowed = true
		c.activeCommandID = ""
	case interaction.EventCancelRequested:
		c.cancelled = true
		if c.usesTaskSession {
			c.paused, c.runFailed = true, true
		}
	case interaction.EventQueuePaused:
		c.paused, c.runFailed = true, true
	case interaction.EventQueueResumed:
		c.paused, c.cancelled = false, false
		if len(c.queue) > 0 {
			c.runFailed = false
		}
	case interaction.EventQueueDiscarded:
		c.paused, c.cancelled = false, false
		for _, prompt := range c.queue {
			c.setCommandState(prompt.ID, interaction.CommandDiscarded)
		}
		c.queue = nil
	case interaction.EventRunCompleted, interaction.EventRunFailed:
		c.runEnded, c.paused = true, false
	}
	// Permission IDs must never repeat after restoring the same runtime session.
	if strings.HasPrefix(event.RequestID, "permission-") {
		c.permissionSequence = max(c.permissionSequence, event.Sequence)
	}
}

func (c *conversationState) removeQueued(commandID string) {
	for i, prompt := range c.queue {
		if prompt.ID == commandID {
			c.queue = append(c.queue[:i], c.queue[i+1:]...)
			return
		}
	}
}

func (c *conversationState) beginCommand(id string) {
	c.removeQueued(id)
	c.activeCommandID = id
	c.setCommandState(id, interaction.CommandRunning)
}

// setCommandState preserves the original prompt hash when a recovered command
// receives a continuation instruction instead of resending its original text.
func (c *conversationState) setCommandState(id string, state interaction.CommandState) {
	record := c.commands[id]
	record.state = state
	c.commands[id] = record
}
