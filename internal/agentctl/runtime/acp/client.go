// Package acp contains the shared ACP stdio communication implementation.
package acp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"

	sdk "github.com/coder/acp-go-sdk"
	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
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
		return agentruntime.NegotiatedCapabilities{}, c.protocolError("initialize ACP connection failed", err)
	}
	return agentruntime.NegotiatedCapabilities{ProtocolVersion: int(response.ProtocolVersion), LoadSession: response.AgentCapabilities.LoadSession, SetSessionMode: true}, nil
}

func (c *client) NewSession(ctx context.Context, workDir string, options agentruntime.ProtocolSessionOptions) (string, error) {
	response, err := c.connection.NewSession(ctx, sdk.NewSessionRequest{Cwd: workDir, McpServers: mcpServers(options.MCPServers), Meta: options.Metadata})
	if err != nil {
		return "", c.protocolError("create ACP session failed", err)
	}
	c.mu.Lock()
	c.sessionID = string(response.SessionId)
	c.mu.Unlock()
	return string(response.SessionId), nil
}

func (c *client) LoadSession(ctx context.Context, id string, workDir string, options agentruntime.ProtocolSessionOptions) error {
	c.mu.Lock()
	c.sessionID = id
	c.mu.Unlock()
	_, err := c.connection.LoadSession(ctx, sdk.LoadSessionRequest{SessionId: sdk.SessionId(id), Cwd: workDir, McpServers: mcpServers(options.MCPServers), Meta: options.Metadata})
	if err != nil {
		return c.protocolError("load ACP session failed", err)
	}
	return nil
}

func mcpServers(servers []runconfig.MCPServer) []sdk.McpServer {
	result := make([]sdk.McpServer, 0, len(servers))
	for _, server := range servers {
		headers := make([]sdk.HttpHeader, 0, len(server.Headers))
		for name, value := range server.Headers {
			headers = append(headers, sdk.HttpHeader{Name: name, Value: value})
		}
		result = append(result, sdk.McpServer{Http: &sdk.McpServerHttpInline{Type: "http", Name: server.Name, Url: server.URL, Headers: headers}})
	}
	return result
}

func (c *client) SetSessionModel(ctx context.Context, id, model string) error {
	return c.selectModel(ctx, id, model)
}

func (c *client) SetSessionMode(ctx context.Context, id, mode string) error {
	_, err := c.connection.SetSessionMode(ctx, sdk.SetSessionModeRequest{SessionId: sdk.SessionId(id), ModeId: sdk.SessionModeId(mode)})
	if err != nil {
		return c.protocolError("set ACP session mode failed", err)
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
		return "", c.protocolErrorKind(kind, "ACP prompt failed", err)
	}
	return agentruntime.StopReason(response.StopReason), nil
}

func (c *client) Cancel(ctx context.Context, id string) error {
	if err := c.connection.Cancel(ctx, sdk.CancelNotification{SessionId: sdk.SessionId(id)}); err != nil {
		return c.protocolError("cancel ACP prompt failed", err)
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
		return callbacks.Update(toolUpdate("tool_call", string(call.ToolCallId), call.Title, string(call.Status), string(call.Kind), call.RawInput, call.RawOutput, call.Locations, call.Content))
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
		if call.Kind != nil {
			value.ToolKind = string(*call.Kind)
		}
		if call.RawInput != nil {
			value.Input, value.Truncated = bounded(call.RawInput, value.Truncated)
		}
		if call.RawOutput != nil {
			value.Output, value.Truncated = bounded(call.RawOutput, value.Truncated)
		}
		if call.Locations != nil {
			value.Locations = locations(call.Locations)
		}
		if call.Content != nil {
			value.Content, value.Truncated = contents(call.Content)
		}
		return callbacks.Update(value)
	default:
		callbacks.Observation(agentruntime.HistoryObservation{Kind: agentruntime.HistoryUnknown})
	}
	return nil
}

// Raw values stay intact until the host redacts credentials and then bounds previews.
func bounded(v any, already bool) (string, bool) {
	if v == nil {
		return "", already
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "[unavailable]", already
	}
	return string(b), already
}

func locations(in []sdk.ToolCallLocation) []interaction.ToolLocation {
	if in == nil {
		return nil
	}
	out := make([]interaction.ToolLocation, 0, len(in))
	for _, l := range in {
		line := 0
		if l.Line != nil {
			line = int(*l.Line)
		}
		out = append(out, interaction.ToolLocation{Path: l.Path, Line: line})
	}
	return out
}

func contents(in []sdk.ToolCallContent) ([]interaction.ToolContent, bool) {
	if in == nil {
		return nil, false
	}
	out := make([]interaction.ToolContent, 0, len(in))
	truncated := false
	for _, c := range in {
		if c.Diff != nil {
			old := ""
			if c.Diff.OldText != nil {
				old = *c.Diff.OldText
			}
			n := false
			out = append(out, interaction.ToolContent{Kind: "diff", Path: c.Diff.Path, OldText: old, NewText: c.Diff.NewText, Truncated: n})
			truncated = truncated || n
		} else if c.Content != nil && c.Content.Content.Text != nil {
			s := c.Content.Content.Text.Text
			n := false
			out = append(out, interaction.ToolContent{Kind: "text", Text: s, Truncated: n})
			truncated = truncated || n
		} else {
			out = append(out, interaction.ToolContent{Kind: "unsupported"})
		}
	}
	return out, truncated
}

func toolUpdate(kind, id, title, status, toolKind string, input, output any, loc []sdk.ToolCallLocation, content []sdk.ToolCallContent) interaction.Update {
	u := interaction.Update{Kind: kind, ToolCallID: id, Title: title, Status: status, ToolKind: toolKind, Locations: locations(loc)}
	u.Input, u.Truncated = bounded(input, u.Truncated)
	var t bool
	u.Output, t = bounded(output, u.Truncated)
	u.Truncated = u.Truncated || t
	u.Content, t = contents(content)
	u.Truncated = u.Truncated || t
	return u
}

func (c *client) protocolError(message string, cause error) error {
	return c.protocolErrorKind(agentruntime.RuntimeFailure, message, cause)
}

func (c *client) protocolErrorKind(kind agentruntime.RuntimeFailureKind, message string, cause error) error {
	var requestError *sdk.RequestError
	if errors.As(cause, &requestError) && requestError.Message != "" {
		detail := strings.TrimSpace(requestError.Message)
		var secrets []string
		if provider, ok := c.callbacks.(interface{ SecretValues() []string }); ok {
			secrets = provider.SecretValues()
		}
		for _, secret := range secrets {
			if secret != "" {
				detail = strings.ReplaceAll(detail, secret, "[redacted]")
			}
		}
		detail = strings.Join(strings.Fields(detail), " ")
		runes := []rune(detail)
		if len(runes) > 400 {
			detail = string(runes[:400]) + "…"
		}
		if detail != "" {
			message += ": " + detail
		}
	}
	return &agentruntime.RuntimeError{Kind: kind, Message: message, Cause: cause}
}

// selectModel prevents ACP sessions from silently retaining a provider default
// when selecting an agent mode does not apply that agent's configured model.
func (c *client) selectModel(ctx context.Context, id, model string) error {
	if model == "" {
		return nil
	}
	response, err := c.connection.SetSessionConfigOption(ctx, sdk.SetSessionConfigOptionRequest{ValueId: &sdk.SetSessionConfigOptionValueId{SessionId: sdk.SessionId(id), ConfigId: "model", Value: sdk.SessionConfigValueId(model)}})
	if err != nil {
		return c.protocolError("select configured ACP model failed", err)
	}
	for _, option := range response.ConfigOptions {
		if option.Select != nil && option.Select.Id == "model" && string(option.Select.CurrentValue) == model {
			return nil
		}
	}
	return &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "runtime did not confirm the configured model"}
}
