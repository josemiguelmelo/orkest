package opencode

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/josemiguelmelo/orkest/internal/agentctl"
	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi"
)

func TestWebSocketConversationReconnect(t *testing.T) {
	service := conversationService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	api, err := httpapi.NewServer(ctx, service)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	defer server.Close()
	address := "ws" + strings.TrimPrefix(server.URL, "http") + "/agent/stream"
	connect := func(url string) *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.Dial(ctx, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		var ready struct {
			Type string `json:"type"`
		}
		if err := wsjson.Read(ctx, conn, &ready); err != nil {
			t.Fatal(err)
		}
		if ready.Type != "ready" {
			t.Fatal("missing ready")
		}
		return conn
	}
	conn := connect(address)
	defer func() { _ = conn.CloseNow() }()
	postCommand(t, ctx, server.URL+"/agent/prompt", map[string]string{"commandId": "rest-command", "text": "permission"}, http.StatusAccepted)
	// An identical REST retry is accepted without creating another prompt.
	postCommand(t, ctx, server.URL+"/agent/prompt", map[string]string{"commandId": "rest-command", "text": "permission"}, http.StatusAccepted)
	postCommand(t, ctx, server.URL+"/agent/prompt", map[string]string{"commandId": "rest-command", "text": "changed"}, http.StatusBadRequest)
	var permission agentctl.Event
	for permission.Type != "permission_requested" {
		var frame struct {
			Type  string         `json:"type"`
			Event agentctl.Event `json:"event"`
		}
		if err := wsjson.Read(ctx, conn, &frame); err != nil {
			t.Fatal(err)
		}
		permission = frame.Event
	}
	_ = conn.CloseNow()
	// Reconnect and replay the pending request, then answer it through REST.
	conn = connect(address + "?afterSequence=0")
	for {
		var frame struct {
			Event agentctl.Event `json:"event"`
		}
		if err := wsjson.Read(ctx, conn, &frame); err != nil {
			t.Fatal(err)
		}
		if frame.Event.Type == "permission_requested" {
			break
		}
	}
	response := agentctl.PermissionResponse{TaskID: "task", RunID: "run", SessionID: permission.SessionID, RequestID: permission.RequestID, OptionID: "allow"}
	postCommand(t, ctx, server.URL+"/agent/permissions/respond", response, http.StatusOK)
	for {
		var frame struct {
			Event agentctl.Event `json:"event"`
		}
		if err := wsjson.Read(ctx, conn, &frame); err != nil {
			t.Fatal(err)
		}
		if frame.Event.Type == "prompt_completed" {
			break
		}
	}
	postCommand(t, ctx, server.URL+"/agent/permissions/respond", response, http.StatusConflict)
	if service.Status().State != "ready" {
		t.Fatal("prompt did not complete")
	}
}

func postCommand(t *testing.T, ctx context.Context, url string, body any, status int) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != status {
		t.Fatalf("%s: got %d, want %d", url, response.StatusCode, status)
	}
	if status == http.StatusAccepted || status == http.StatusOK {
		var acknowledgement struct {
			Status    string `json:"status"`
			CommandID string `json:"commandId"`
			RequestID string `json:"requestId"`
		}
		if err := json.NewDecoder(response.Body).Decode(&acknowledgement); err != nil {
			t.Fatal(err)
		}
		expected := "accepted"
		if strings.HasSuffix(url, "/cancel") {
			expected = "cancel_requested"
		}
		if strings.HasSuffix(url, "/permissions/respond") {
			expected = "permission_response_recorded"
		}
		var submitted map[string]any
		if err := json.Unmarshal(data, &submitted); err != nil {
			t.Fatal(err)
		}
		if acknowledgement.Status != expected {
			t.Fatalf("unexpected acknowledgement: %+v", acknowledgement)
		}
		if id, ok := submitted["commandId"]; ok && acknowledgement.CommandID != id {
			t.Fatal("command identity missing from acknowledgement")
		}
		if id, ok := submitted["requestId"]; ok && acknowledgement.RequestID != id {
			t.Fatal("permission identity missing from acknowledgement")
		}
	}
}

func TestRESTPromptAndCancelWithoutWebSocket(t *testing.T) {
	service := conversationService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	api, err := httpapi.NewServer(ctx, service)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.Handler())
	defer server.Close()
	_, events, stop, err := service.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	postCommand(t, ctx, server.URL+"/agent/prompt", map[string]string{"commandId": "rest-only", "text": "wait"}, http.StatusAccepted)
	nextEvent(t, events, "update")
	postCommand(t, ctx, server.URL+"/agent/cancel", map[string]string{"commandId": "wrong"}, http.StatusBadRequest)
	postCommand(t, ctx, server.URL+"/agent/cancel", map[string]string{"commandId": "rest-only"}, http.StatusAccepted)
	completed := nextEvent(t, events, "prompt_completed")
	if completed.StopReason != "cancelled" {
		t.Fatalf("unexpected result: %+v", completed)
	}
	// A later WebSocket observes the durable REST command history.
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/agent/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.CloseNow() }()
	for {
		var frame struct {
			Event agentctl.Event `json:"event"`
		}
		if err := wsjson.Read(ctx, conn, &frame); err != nil {
			t.Fatal(err)
		}
		if frame.Event.Type == "prompt_completed" {
			if frame.Event.CommandID != "rest-only" {
				t.Fatal("wrong command replayed")
			}
			break
		}
	}
}
