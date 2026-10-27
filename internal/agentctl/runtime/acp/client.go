// Package acp contains the shared ACP stdio communication implementation.
package acp

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

type Factory struct{}

func (Factory) Connect(_ context.Context, input io.WriteCloser, output io.ReadCloser, callbacks agentruntime.ProtocolCallbacks) (agentruntime.ProtocolClient, error) {
	client := &client{callbacks: callbacks}
	connection := sdk.NewClientSideConnection(client, input, output)
	connection.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	client.connection = connection
	return client, nil
}

type client struct {
	mu         sync.Mutex
	callbacks  agentruntime.ProtocolCallbacks
	connection *sdk.ClientSideConnection
	sessionID  string
}

func (c *client) Initialize(ctx context.Context) (agentruntime.NegotiatedCapabilities, error) {
	response, err := c.connection.Initialize(ctx, sdk.InitializeRequest{ProtocolVersion: 1})
	if err != nil {
		return agentruntime.NegotiatedCapabilities{}, protocolError("initialize ACP connection failed", err)
	}
	return agentruntime.NegotiatedCapabilities{ProtocolVersion: int(response.ProtocolVersion), LoadSession: response.AgentCapabilities.LoadSession, SetSessionMode: true}, nil
}

func (c *client) NewSession(ctx context.Context, workDir string) (string, error) {
	response, err := c.connection.NewSession(ctx, sdk.NewSessionRequest{Cwd: workDir, McpServers: []sdk.McpServer{}})
	if err != nil {
		return "", protocolError("create ACP session failed", err)
	}
	c.mu.Lock()
	c.sessionID = string(response.SessionId)
	c.mu.Unlock()
	return string(response.SessionId), nil
}

func (c *client) LoadSession(ctx context.Context, id string, workDir string) error {
	c.mu.Lock()
	c.sessionID = id
	c.mu.Unlock()
	_, err := c.connection.LoadSession(ctx, sdk.LoadSessionRequest{SessionId: sdk.SessionId(id), Cwd: workDir, McpServers: []sdk.McpServer{}})
	if err != nil {
		return protocolError("load ACP session failed", err)
	}
	return nil
}

func (c *client) SetSessionMode(ctx context.Context, id, mode string) error {
	_, err := c.connection.SetSessionMode(ctx, sdk.SetSessionModeRequest{SessionId: sdk.SessionId(id), ModeId: sdk.SessionModeId(mode)})
	if err != nil {
		return protocolError("set ACP session mode failed", err)
	}
	return nil
}

func (c *client) Prompt(ctx context.Context, id, text string) (agentruntime.StopReason, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	response, err := c.connection.Prompt(ctx, sdk.PromptRequest{SessionId: sdk.SessionId(id), Prompt: []sdk.ContentBlock{sdk.TextBlock(text)}})
	if err != nil {
		kind := agentruntime.RuntimeFailure
		select {
		case <-c.connection.Done():
			kind = agentruntime.RuntimeInterrupted
		default:
		}
		return "", &agentruntime.RuntimeError{Kind: kind, Message: "ACP prompt failed", Cause: err}
	}
	return agentruntime.StopReason(response.StopReason), nil
}

func (c *client) Cancel(ctx context.Context, id string) error {
	if err := c.connection.Cancel(ctx, sdk.CancelNotification{SessionId: sdk.SessionId(id)}); err != nil {
		return protocolError("cancel ACP prompt failed", err)
	}
	return nil
}

func (c *client) Done() <-chan struct{} { return c.connection.Done() }
func (c *client) Err() error {
	select {
	case <-c.Done():
		return &agentruntime.RuntimeError{Kind: agentruntime.RuntimeInterrupted, Message: "ACP connection terminated"}
	default:
		return nil
	}
}

func (c *client) RequestPermission(ctx context.Context, request sdk.RequestPermissionRequest) (sdk.RequestPermissionResponse, error) {
	permission := interaction.Permission{ToolCallID: string(request.ToolCall.ToolCallId)}
	if request.ToolCall.Title != nil {
		permission.Title = *request.ToolCall.Title
	}
	for _, option := range request.Options {
		permission.Options = append(permission.Options, interaction.PermissionOption{ID: string(option.OptionId), Name: option.Name, Kind: string(option.Kind)})
	}
	c.mu.Lock()
	matches := c.sessionID == string(request.SessionId)
	callbacks := c.callbacks
	c.mu.Unlock()
	if !matches {
		return sdk.RequestPermissionResponse{}, errors.New("no matching ACP session")
	}
	selected, err := callbacks.Permission(ctx, permission)
	if err != nil {
		return sdk.RequestPermissionResponse{Outcome: sdk.NewRequestPermissionOutcomeCancelled()}, nil
	}
	for _, option := range request.Options {
		if string(option.OptionId) == selected {
			return sdk.RequestPermissionResponse{Outcome: sdk.NewRequestPermissionOutcomeSelected(option.OptionId)}, nil
		}
	}
	return sdk.RequestPermissionResponse{}, errors.New("permission option unavailable")
}

func (c *client) SessionUpdate(_ context.Context, notification sdk.SessionNotification) error {
	c.mu.Lock()
	matches := c.sessionID == string(notification.SessionId)
	callbacks := c.callbacks
	c.mu.Unlock()
	if !matches {
		return nil
	}
	update := notification.Update
	switch {
	case update.UserMessageChunk != nil && update.UserMessageChunk.Content.Text != nil:
		callbacks.Observation(agentruntime.HistoryObservation{Kind: agentruntime.HistoryUserChunk, Text: update.UserMessageChunk.Content.Text.Text})
	case update.AgentMessageChunk != nil:
		content := update.AgentMessageChunk.Content
		if content.Text != nil {
			return callbacks.Update(interaction.Update{Kind: "text", Text: content.Text.Text})
		}
		callbacks.Observation(agentruntime.HistoryObservation{Kind: agentruntime.HistoryUnknown})
	case update.ToolCall != nil:
		call := update.ToolCall
		callbacks.Observation(agentruntime.HistoryObservation{Kind: agentruntime.HistoryToolActivity})
		return callbacks.Update(interaction.Update{Kind: "tool_call", ToolCallID: string(call.ToolCallId), Title: call.Title, Status: string(call.Status)})
	case update.ToolCallUpdate != nil:
		call := update.ToolCallUpdate
		callbacks.Observation(agentruntime.HistoryObservation{Kind: agentruntime.HistoryToolActivity})
		value := interaction.Update{Kind: "tool_call_update", ToolCallID: string(call.ToolCallId)}
		if call.Title != nil {
			value.Title = *call.Title
		}
		if call.Status != nil {
			value.Status = string(*call.Status)
		}
		return callbacks.Update(value)
	default:
		callbacks.Observation(agentruntime.HistoryObservation{Kind: agentruntime.HistoryUnknown})
	}
	return nil
}

func protocolError(message string, cause error) error {
	return &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: message, Cause: cause}
}
