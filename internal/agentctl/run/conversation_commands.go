package run

import (
	"context"
	"crypto/sha256"
	"strings"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

func (c *conversation) CommandStatus(commandID string) (interaction.CommandStatus, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	record, ok := c.state.commands[commandID]
	if !ok {
		return interaction.CommandStatus{}, interaction.ErrCommand
	}
	return interaction.CommandStatus{CommandID: commandID, Status: record.state}, nil
}

// Prompt durably accepts an idempotent command. Execution belongs to the
// conversation and continues after the requesting connection disappears.
func (c *conversation) Prompt(commandID, text string) error {
	command := interaction.Command{ID: commandID, Text: text}
	logAttrs := c.logAttrs(commandID)
	c.logger.Info("prompt received", logAttrs...)
	if strings.TrimSpace(commandID) == "" || len(commandID) > 200 || len(text) > interaction.MaxPromptBytes || strings.TrimSpace(text) == "" {
		c.logger.Warn("prompt rejected", append(logAttrs, "reason", "invalid_command")...)
		return interaction.ErrCommand
	}
	c.turnMu.Lock()
	defer c.turnMu.Unlock()
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	if duplicate, err := c.checkPromptAdmission(command); duplicate || err != nil {
		return err
	}
	if err := c.ensureJournalUnderEventLock(); err != nil {
		c.logger.Error("prompt journal setup failed", append(logAttrs, "reason", "journal_unavailable")...)
		return err
	}
	// Journal replay may reveal an earlier acceptance, a paused queue, or a
	// terminal run. Shutdown may also have begun during the storage operation.
	if duplicate, err := c.checkPromptAdmission(command); duplicate || err != nil {
		return err
	}
	if err := c.commitEventUnderEventLock(promptAcceptedEvent(commandID, text)); err != nil {
		c.logger.Error("prompt acceptance persistence failed", append(logAttrs, "reason", "journal_write_failed")...)
		return err
	}
	c.mu.RLock()
	stopped := c.closed || c.ctx.Err() != nil
	c.mu.RUnlock()
	if stopped {
		c.interrupt()
		_ = c.commitEventUnderEventLock(promptResultEvent(commandID, "", context.Canceled))
		return interaction.ErrConversation
	}
	return c.startNextUnderTurnAndEventLocks()
}

func (c *conversation) checkPromptAdmission(command interaction.Command) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed || c.session == nil || c.executionState == StateFailed {
		return false, interaction.ErrConversation
	}
	if previous, exists := c.state.commands[command.ID]; exists {
		if previous.contentHash == sha256.Sum256([]byte(command.Text)) {
			return true, nil
		}
		return false, interaction.ErrCommand
	}
	if c.executionState == StateInterrupted {
		return false, interaction.ErrConversationInterrupted
	}
	select {
	case <-c.session.Done():
		return false, interaction.ErrConversation
	default:
	}
	if c.ctx.Err() != nil {
		return false, interaction.ErrConversation
	}
	if len(c.state.commands) >= interaction.MaxCommands {
		return false, interaction.ErrCommandLimit
	}
	if c.state.runEnded || c.state.paused {
		return false, interaction.ErrConversationInterrupted
	}
	if c.state.activeCommandID != "" && !c.state.usesTaskSession {
		return false, interaction.ErrConversationBusy
	}
	return false, nil
}

func (c *conversation) commandObserver(commandID string) agentruntime.PromptObserver {
	return agentruntime.PromptObserver{
		Update: func(update interaction.Update) error {
			c.eventMu.Lock()
			defer c.eventMu.Unlock()
			c.mu.RLock()
			accepts := !c.closed && c.executionState != StateInterrupted && c.state.activeCommandID == commandID
			c.mu.RUnlock()
			if !accepts {
				return interaction.ErrConversation
			}
			return c.commitEventUnderEventLock(updateEvent(commandID, update))
		},
		Permission: func(ctx context.Context, permission interaction.Permission) (string, error) {
			return c.waitPermission(ctx, commandID, permission)
		},
	}
}

func (c *conversation) runPrompt(runtime agentruntime.RuntimeSession, command interaction.Command) {
	defer c.work.Done()
	startedAt := time.Now()
	logAttrs := c.logAttrs(command.ID)
	c.logger.Info("runtime prompt started", logAttrs...)
	result, err := runtime.Prompt(c.ctx, command, c.commandObserver(command.ID))
	if !c.finishPrompt(command.ID, result, err) {
		return
	}
	attrs := append(logAttrs, "duration_ms", time.Since(startedAt).Milliseconds())
	if err != nil {
		c.logger.Warn("runtime prompt finished with failure", append(attrs, "reason", "provider_error")...)
	} else {
		c.logger.Info("runtime prompt finished", append(attrs, "has_stop_reason", result != "")...)
	}
}

// finishPrompt serializes result persistence and queue scheduling with admission
// and cancellation. False means persistence failed or recovery owns the turn.
func (c *conversation) finishPrompt(commandID string, reason agentruntime.StopReason, promptErr error) bool {
	c.turnMu.Lock()
	defer c.turnMu.Unlock()
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.mu.Lock()
	interrupted := c.isInfrastructureInterruptionUnderStateLock(promptErr)
	if interrupted {
		c.executionState = StateInterrupted
	}
	for id, permission := range c.pending {
		if permission.commandID == commandID {
			delete(c.pending, id)
		}
	}
	usesTaskSession := c.state.usesTaskSession
	c.mu.Unlock()
	if interrupted {
		// Infrastructure interruption leaves delivery unresolved for reconciliation.
		_ = c.commitEventUnderEventLock(interaction.Event{Type: interaction.EventRuntimeStopped, CommandID: commandID, Error: "runtime stopped; restoring durable session"})
		return false
	}
	if promptErr != nil {
		if usesTaskSession {
			c.setExecutionState(StateInterrupted)
		} else {
			c.interrupt()
		}
	}
	if err := c.commitEventUnderEventLock(promptResultEvent(commandID, reason, promptErr)); err != nil {
		return false
	}
	c.mu.RLock()
	paused := promptErr != nil || c.state.cancelled || reason == "cancelled" || c.executionState == StateInterrupted
	c.mu.RUnlock()
	if usesTaskSession {
		if paused {
			c.setExecutionState(StateInterrupted)
			_ = c.commitEventUnderEventLock(interaction.Event{Type: interaction.EventQueuePaused})
		} else {
			c.setExecutionState(StateReady)
			_ = c.startNextUnderTurnAndEventLocks()
		}
	} else if c.Snapshot().State != StateInterrupted {
		c.setExecutionState(StateReady)
	}
	return true
}

// CancelPrompt retains turnMu through delivery so completion cannot advance the
// queue and accidentally deliver cancellation to the next command. Status and
// runtime updates do not need turnMu and remain responsive.
func (c *conversation) CancelPrompt(commandID string) error {
	c.logger.Info("prompt cancellation requested", c.logAttrs(commandID)...)
	c.turnMu.Lock()
	defer c.turnMu.Unlock()
	runtime, err := c.prepareCancellation(commandID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()
	if err := runtime.Cancel(ctx); err != nil {
		c.interrupt()
		return interaction.ErrCancellation
	}
	return nil
}

func (c *conversation) prepareCancellation(commandID string) (agentruntime.RuntimeSession, error) {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.mu.RLock()
	runtime := c.session
	valid := !c.closed && runtime != nil && commandID != "" && c.state.activeCommandID == commandID
	c.mu.RUnlock()
	if !valid {
		return nil, interaction.ErrCommand
	}
	if err := c.commitEventUnderEventLock(cancelRequestedEvent(commandID)); err != nil {
		return nil, err
	}
	return runtime, nil
}

func runtimeStopped(session agentruntime.RuntimeSession, err error) bool {
	if agentruntime.IsRuntimeInterrupted(err) {
		return true
	}
	select {
	case <-session.Done():
		return true
	default:
		return false
	}
}

// isInfrastructureInterruptionUnderStateLock distinguishes recovery from ordinary
// failure and user cancellation. The caller holds mu.
func (c *conversation) isInfrastructureInterruptionUnderStateLock(err error) bool {
	return err != nil && c.state.usesTaskSession && !c.state.cancelled &&
		(c.closed || c.ctx.Err() != nil || runtimeStopped(c.session, err))
}
