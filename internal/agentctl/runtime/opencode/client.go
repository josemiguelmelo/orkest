package opencode

import (
	"context"
	"errors"
	"sync"

	"github.com/josemiguelmelo/orkest/internal/agentctl"

	acp "github.com/coder/acp-go-sdk"
)

// runtimeClient advertises no filesystem or terminal capabilities.
type runtimeClient struct {
	mu              sync.Mutex
	observer        *agentctl.Observer
	sessionID       string
	restoring       bool
	replayUser      string
	replayUserChunk bool
	replayTools     bool
}

func (*runtimeClient) ReadTextFile(context.Context, acp.ReadTextFileRequest) (acp.ReadTextFileResponse, error) {
	return acp.ReadTextFileResponse{}, errors.New("client capability unavailable")
}

func (*runtimeClient) WriteTextFile(context.Context, acp.WriteTextFileRequest) (acp.WriteTextFileResponse, error) {
	return acp.WriteTextFileResponse{}, errors.New("client capability unavailable")
}

func (c *runtimeClient) RequestPermission(ctx context.Context, request acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	c.mu.Lock()
	observer, sessionID := c.observer, c.sessionID
	c.mu.Unlock()
	if observer == nil || string(request.SessionId) != sessionID {
		return acp.RequestPermissionResponse{}, errors.New("no matching active prompt")
	}
	permission := agentctl.Permission{ToolCallID: string(request.ToolCall.ToolCallId)}
	if request.ToolCall.Title != nil {
		permission.Title = *request.ToolCall.Title
	}
	for _, option := range request.Options {
		permission.Options = append(permission.Options, agentctl.PermissionOption{ID: string(option.OptionId), Name: option.Name, Kind: string(option.Kind)})
	}
	selected, err := observer.Permission(ctx, permission)
	if err != nil {
		return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeCancelled()}, nil
	}
	for _, option := range request.Options {
		if string(option.OptionId) == selected {
			return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeSelected(option.OptionId)}, nil
		}
	}
	return acp.RequestPermissionResponse{}, errors.New("permission option unavailable")
}

func (*runtimeClient) CreateTerminal(context.Context, acp.CreateTerminalRequest) (acp.CreateTerminalResponse, error) {
	return acp.CreateTerminalResponse{}, errors.New("client capability unavailable")
}

func (*runtimeClient) KillTerminal(context.Context, acp.KillTerminalRequest) (acp.KillTerminalResponse, error) {
	return acp.KillTerminalResponse{}, errors.New("client capability unavailable")
}

func (*runtimeClient) TerminalOutput(context.Context, acp.TerminalOutputRequest) (acp.TerminalOutputResponse, error) {
	return acp.TerminalOutputResponse{}, errors.New("client capability unavailable")
}

func (*runtimeClient) ReleaseTerminal(context.Context, acp.ReleaseTerminalRequest) (acp.ReleaseTerminalResponse, error) {
	return acp.ReleaseTerminalResponse{}, errors.New("client capability unavailable")
}

func (*runtimeClient) WaitForTerminalExit(context.Context, acp.WaitForTerminalExitRequest) (acp.WaitForTerminalExitResponse, error) {
	return acp.WaitForTerminalExitResponse{}, errors.New("client capability unavailable")
}

func (c *runtimeClient) SessionUpdate(_ context.Context, notification acp.SessionNotification) error {
	c.mu.Lock()
	observer, sessionID := c.observer, c.sessionID
	if c.restoring && string(notification.SessionId) == sessionID {
		c.replayUpdate(notification.Update)
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()
	if observer == nil || string(notification.SessionId) != sessionID {
		return nil
	}
	var update agentctl.Update
	switch {
	case notification.Update.AgentMessageChunk != nil:
		content := notification.Update.AgentMessageChunk.Content
		if content.Text == nil {
			return nil
		}
		update = agentctl.Update{Kind: "text", Text: content.Text.Text}
	case notification.Update.ToolCall != nil:
		call := notification.Update.ToolCall
		update = agentctl.Update{Kind: "tool_call", ToolCallID: string(call.ToolCallId), Title: call.Title, Status: string(call.Status)}
	case notification.Update.ToolCallUpdate != nil:
		call := notification.Update.ToolCallUpdate
		update = agentctl.Update{Kind: "tool_call_update", ToolCallID: string(call.ToolCallId)}
		if call.Title != nil {
			update.Title = *call.Title
		}
		if call.Status != nil {
			update.Status = string(*call.Status)
		}
	default:
		return nil
	}
	return observer.Update(update)
}

func (c *runtimeClient) replayUpdate(update acp.SessionUpdate) {
	if update.UserMessageChunk != nil && update.UserMessageChunk.Content.Text != nil {
		if !c.replayUserChunk {
			c.replayUser = ""
			c.replayTools = false
		}
		c.replayUser += update.UserMessageChunk.Content.Text.Text
		c.replayUserChunk = true
		return
	}
	c.replayUserChunk = false
	if update.ToolCall != nil || update.ToolCallUpdate != nil {
		c.replayTools = true
	}
}
