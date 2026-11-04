package run

import (
	"context"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

// ReplayAndReconcile replays the journal, reconciles unresolved delivery, and schedules any
// accepted queued work. Admission remains serialized; state reads never hold mu
// over journal or runtime I/O.
func (c *conversation) ReplayAndReconcile(ctx context.Context) error {
	c.turnMu.Lock()
	defer c.turnMu.Unlock()
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.mu.Lock()
	c.replayExistingJournal = true
	c.mu.Unlock()
	if err := c.ensureJournalUnderEventLock(); err != nil {
		return err
	}
	if c.Snapshot().RunEnded {
		return nil
	}
	history, err := c.journal.Replay(0)
	if err != nil {
		return err
	}
	for _, command := range c.unresolvedCommands(history) {
		decision, err := c.session.Reconcile(ctx, command)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !c.Snapshot().QueuePaused && err == nil && decision.Action == agentruntime.RecoveryContinue {
			text := "Continue the interrupted response to the previous user message from the restored conversation. No tool execution was recorded for that turn. Check the current task state before taking any action."
			if err := c.commitEventUnderEventLock(interaction.Event{Type: interaction.EventPromptContinued, CommandID: command.ID, Text: text}); err != nil {
				return err
			}
		} else {
			if err := c.commitEventUnderEventLock(interaction.Event{Type: interaction.EventPromptInterrupted, CommandID: command.ID, Error: "runtime delivery or unfinished effects require reconciliation; original prompt was not resent"}); err != nil {
				return err
			}
		}
	}
	if err := c.commitEventUnderEventLock(interaction.Event{Type: interaction.EventSessionRestored}); err != nil {
		return err
	}
	c.mu.RLock()
	paused := c.state.paused
	hasCommands := len(c.state.queue) != 0 || len(c.state.commands) != 0
	c.mu.RUnlock()
	if paused {
		c.setExecutionState(StateInterrupted)
		return c.commitEventUnderEventLock(interaction.Event{Type: interaction.EventQueuePaused})
	}
	if hasCommands {
		return c.startNextUnderTurnAndEventLocks()
	}
	return nil
}

func (c *conversation) unresolvedCommands(history []interaction.Event) []interaction.Command {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var commands []interaction.Command
	for commandID, record := range c.state.commands {
		if record.state != interaction.CommandRunning {
			continue
		}
		command := interaction.Command{ID: commandID}
		for _, event := range history {
			if (event.Type == interaction.EventPromptAccepted || event.Type == interaction.EventPromptContinued) && event.CommandID == commandID {
				command.Text = event.Text
			}
		}
		commands = append(commands, command)
	}
	return commands
}

func journalRunEnded(stateDir string, config runconfig.RunConfig) (bool, error) {
	journal, events, err := loadJournal(stateDir, config, true)
	if err != nil {
		return false, err
	}
	defer func() { _ = journal.Close() }()
	state := conversationState{usesTaskSession: config.TaskSessionID != ""}
	for _, event := range events {
		if err := state.validate(event); err != nil {
			return false, err
		}
		state.apply(event)
	}
	return state.runEnded, nil
}
