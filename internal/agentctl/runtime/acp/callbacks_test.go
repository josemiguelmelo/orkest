package acp

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

type recordedCallbacks struct {
	updates      []interaction.Update
	observations []agentruntime.HistoryObservation
	permission   interaction.Permission
	option       string
	err          error
}

func (c *recordedCallbacks) Update(update interaction.Update) error {
	c.updates = append(c.updates, update)
	return c.err
}

func (c *recordedCallbacks) Observation(observation agentruntime.HistoryObservation) {
	c.observations = append(c.observations, observation)
}

func (c *recordedCallbacks) Permission(_ context.Context, permission interaction.Permission) (string, error) {
	c.permission = permission
	return c.option, c.err
}

func TestSessionUpdatesNormalizeAndEnforceSessionIdentity(t *testing.T) {
	callbacks := &recordedCallbacks{}
	client := &client{sessionID: "session", callbacks: callbacks}
	for _, packet := range []string{
		`{"sessionId":"other","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"wrong session"}}}`,
		`{"sessionId":"session","update":{"sessionUpdate":"user_message_chunk","content":{"type":"text","text":"original"}}}`,
		`{"sessionId":"session","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"reply"}}}`,
		`{"sessionId":"session","update":{"sessionUpdate":"tool_call","toolCallId":"tool","title":"Edit file","status":"pending"}}`,
	} {
		var notification sdk.SessionNotification
		if err := json.Unmarshal([]byte(packet), &notification); err != nil {
			t.Fatal(err)
		}
		if err := client.SessionUpdate(context.Background(), notification); err != nil {
			t.Fatal(err)
		}
	}
	if want := []interaction.Update{{Kind: "text", Text: "reply"}, {Kind: "tool_call", ToolCallID: "tool", Title: "Edit file", Status: "pending"}}; !reflect.DeepEqual(callbacks.updates, want) {
		t.Fatalf("unexpected live updates: %+v", callbacks.updates)
	}
	if want := []agentruntime.HistoryObservation{{Kind: agentruntime.HistoryUserChunk, Text: "original"}, {Kind: agentruntime.HistoryToolActivity}}; !reflect.DeepEqual(callbacks.observations, want) {
		t.Fatalf("unexpected recovery observations: %+v", callbacks.observations)
	}
}

func TestPermissionSelectionIsBoundToSessionAndOfferedOptions(t *testing.T) {
	var request sdk.RequestPermissionRequest
	if err := json.Unmarshal([]byte(`{"sessionId":"session","toolCall":{"toolCallId":"tool","title":"Edit file"},"options":[{"optionId":"reject","name":"Reject once","kind":"reject_once"}]}`), &request); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, session, option string
		failure               error
		wantError             bool
		wantOutcome           sdk.RequestPermissionOutcome
	}{
		{name: "selected", session: "session", option: "reject", wantOutcome: sdk.NewRequestPermissionOutcomeSelected("reject")},
		{name: "handler failed", session: "session", failure: errors.New("unavailable"), wantOutcome: sdk.NewRequestPermissionOutcomeCancelled()},
		{name: "wrong session", session: "other", option: "reject", wantError: true},
		{name: "unoffered option", session: "session", option: "allow", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			callbacks := &recordedCallbacks{option: tc.option, err: tc.failure}
			client := &client{sessionID: tc.session, callbacks: callbacks}
			response, err := client.RequestPermission(context.Background(), request)
			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected permission error: %v", err)
			}
			if !tc.wantError && !reflect.DeepEqual(response.Outcome, tc.wantOutcome) {
				t.Fatalf("unexpected permission outcome: %+v", response.Outcome)
			}
			if tc.session == "other" && callbacks.permission.ToolCallID != "" {
				t.Fatal("forwarded permission for another session")
			}
		})
	}
}
