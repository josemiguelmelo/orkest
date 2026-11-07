package run

import (
	"context"
	"fmt"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	runmodel "github.com/josemiguelmelo/orkest/internal/agentctl/run/model"
)

type pendingPermission struct {
	permission interaction.Permission
	ctx        context.Context
	commandID  string
	response   chan string
	answered   bool
}

// registerPermission serializes request persistence with response admission.
func (c *conversation) registerPermission(ctx context.Context, commandID string, permission interaction.Permission) (string, *pendingPermission, error) {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.mu.Lock()
	if c.closed || c.executionState == runmodel.StateInterrupted || c.state.activeCommandID != commandID || ctx.Err() != nil {
		c.mu.Unlock()
		return "", nil, interaction.ErrConversation
	}
	c.state.permissionSequence++
	id := fmt.Sprintf("permission-%d", c.state.permissionSequence)
	c.mu.Unlock()
	pending := &pendingPermission{permission: permission, commandID: commandID, response: make(chan string, 1), ctx: ctx}
	if err := c.commitEventUnderEventLock(permissionRequestedEvent(commandID, id, permission)); err != nil {
		return "", nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.ctx.Err() != nil || ctx.Err() != nil {
		return "", nil, interaction.ErrConversation
	}
	c.pending[id] = pending
	c.executionState = runmodel.StateWaitingPermission
	return id, pending, nil
}

func (c *conversation) waitPermission(ctx context.Context, commandID string, permission interaction.Permission) (string, error) {
	id, pending, err := c.registerPermission(ctx, commandID, permission)
	if err != nil {
		return "", err
	}
	defer c.removePendingPermission(id)
	select {
	case option := <-pending.response:
		return option, nil
	case <-ctx.Done():
		return "", ctx.Err()
	case <-c.ctx.Done():
		return "", c.ctx.Err()
	}
}

func (c *conversation) removePendingPermission(id string) {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, id)
	c.refreshWaitingStateUnderLock()
}

func (c *conversation) RespondPermission(response interaction.PermissionResponse) error {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	pending, err := c.validatePermissionResponse(response)
	if err != nil {
		return err
	}
	// Recording delivery intent does not prove the runtime accepted the answer.
	if err := c.commitEventUnderEventLock(permissionResponseEvent(pending.commandID, response)); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.ctx.Err() != nil || pending.ctx.Err() != nil {
		return interaction.ErrPermission
	}
	pending.answered = true
	pending.response <- response.OptionID
	return nil
}

func (c *conversation) validatePermissionResponse(response interaction.PermissionResponse) (*pendingPermission, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed || c.executionState == runmodel.StateInterrupted || c.config == nil || c.session == nil {
		return nil, interaction.ErrPermission
	}
	if response.TaskID != c.config.TaskID || response.RunID != c.config.RunID || response.SessionID != c.session.SessionID() {
		return nil, interaction.ErrPermission
	}
	pending, ok := c.pending[response.RequestID]
	if !ok || pending.answered || pending.ctx.Err() != nil {
		return nil, interaction.ErrPermission
	}
	for _, option := range pending.permission.Options {
		if option.ID == response.OptionID {
			return pending, nil
		}
	}
	return nil, interaction.ErrPermission
}
