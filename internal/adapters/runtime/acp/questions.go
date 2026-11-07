package acp

import (
	"context"
	"encoding/json"
	"errors"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	"github.com/josemiguelmelo/orkest/internal/platform/form"
)

func (c *client) initializeRequest() sdk.InitializeRequest {
	request := sdk.InitializeRequest{ProtocolVersion: 1}
	if _, ok := c.callbacks.(agentruntime.QuestionCallbacks); ok {
		request.ClientCapabilities.Elicitation = &sdk.ElicitationCapabilities{Form: &sdk.ElicitationFormCapabilities{}}
	}
	return request
}

// handleRequest retains the native scope fields missing from the pinned SDK's
// elicitation union. All framing, cancellation, and request ordering stay in SDK.
func (c *client) handleRequest(ctx context.Context, method string, raw json.RawMessage) (any, *sdk.RequestError) {
	var result any
	var err error
	switch method {
	case "elicitation/create":
		result, err = c.elicit(ctx, raw)
	case "session/request_permission":
		var request sdk.RequestPermissionRequest
		if json.Unmarshal(raw, &request) != nil {
			return nil, sdk.NewInvalidParams("invalid permission request")
		}
		result, err = c.RequestPermission(ctx, request)
	case "session/update":
		var request sdk.SessionNotification
		if json.Unmarshal(raw, &request) != nil {
			return nil, sdk.NewInvalidParams("invalid session update")
		}
		err = c.SessionUpdate(ctx, request)
	default:
		return nil, sdk.NewMethodNotFound(method)
	}
	if err != nil {
		return nil, sdk.NewInvalidParams("request unavailable, unsupported, or outside the active session")
	}
	return result, nil
}

func (c *client) elicit(ctx context.Context, raw json.RawMessage) (interaction.QuestionAnswer, error) {
	var request struct {
		Mode       string          `json:"mode"`
		SessionID  string          `json:"sessionId"`
		ToolCallID string          `json:"toolCallId"`
		Message    string          `json:"message"`
		Schema     json.RawMessage `json:"requestedSchema"`
	}
	if json.Unmarshal(raw, &request) != nil || request.Mode != "form" {
		return interaction.QuestionAnswer{}, form.ErrInvalid
	}
	c.mu.Lock()
	matches := request.SessionID != "" && request.SessionID == c.sessionID
	callbacks, ok := c.callbacks.(agentruntime.QuestionCallbacks)
	c.mu.Unlock()
	if !matches || !ok {
		return interaction.QuestionAnswer{}, errors.New("no matching question session")
	}
	if _, err := form.Compile(request.Schema); err != nil {
		return interaction.QuestionAnswer{}, err
	}
	answer, err := callbacks.Question(ctx, interaction.Question{ToolCallID: request.ToolCallID, Message: request.Message, Schema: request.Schema})
	if err != nil {
		return interaction.QuestionAnswer{Action: "cancel"}, nil
	}
	return answer, nil
}
