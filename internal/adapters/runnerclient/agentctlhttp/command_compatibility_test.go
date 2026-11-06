package agentctlhttp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
)

// Literals capture the previous HTTP representation, not a new-code round trip.
func TestPreviousV1CommandAndStatusPayloads(t *testing.T) {
	requests := make(chan struct{ path, body string }, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/info" {
			_, _ = io.WriteString(w, `{"organizationId":"org","workspaceId":"workspace","taskId":"task","runId":"run","configured":true,"executionAvailable":true,"processAlive":true,"state":"waiting_for_permission","sessionId":"native","taskSessionId":"conversation","activeCommand":"command","queuePaused":true,"queueDepth":2,"runEnded":false,"recoveryBlocked":true}`)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("command transport changed")
		}
		requests <- struct{ path, body string }{r.URL.Path, string(body)}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client := NewHTTPAgentctlClient(server.Client())
	if err := client.Prompt(ctx, server.URL, "command", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := client.CancelPrompt(ctx, server.URL, "command"); err != nil {
		t.Fatal(err)
	}
	if err := client.RespondPermission(ctx, server.URL, orchestration.PermissionResponse{TaskID: "task", RunID: "run", SessionID: "native", RequestID: "request", OptionID: "allow-once"}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []struct{ path, body string }{
		{"/agent/prompt", `{"commandId":"command","text":"hello"}`},
		{"/agent/cancel", `{"commandId":"command"}`},
		{"/agent/permissions/respond", `{"taskId":"task","runId":"run","sessionId":"native","requestId":"request","optionId":"allow-once"}`},
	} {
		select {
		case actual := <-requests:
			var got, want any
			if err := json.Unmarshal([]byte(actual.body), &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(expected.body), &want); err != nil {
				t.Fatal(err)
			}
			if actual.path != expected.path || !reflect.DeepEqual(got, want) {
				t.Fatalf("wire command changed: %+v", actual)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	status, err := client.Status(ctx, server.URL)
	if err != nil || status.OrganizationID != "org" || status.WorkspaceID != "workspace" || status.TaskID != "task" || status.RunID != "run" || status.SessionID != "native" || status.TaskSessionID != "conversation" || status.State != orchestration.StateWaitingPermission || !status.Configured || !status.ExecutionAvailable || !status.ProcessAlive || !status.QueuePaused || status.QueueDepth != 2 || status.RunEnded || !status.RecoveryBlocked || status.ActiveCommandID != "command" {
		t.Fatalf("v1 status lost fields: %+v %v", status, err)
	}
}
