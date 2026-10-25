package opencode

import (
	"context"
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
	if err := wsjson.Write(ctx, conn, map[string]string{"type": "prompt", "requestId": "ws-command", "text": "permission"}); err != nil {
		t.Fatal(err)
	}
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
	// Reconnect and replay the pending request, then answer it through the stream.
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
	if err := wsjson.Write(ctx, conn, map[string]any{"type": "permission_response", "requestId": "answer-1", "permission": response}); err != nil {
		t.Fatal(err)
	}
	completed, ack := false, false
	for !completed || !ack {
		var frame struct {
			Type  string         `json:"type"`
			Event agentctl.Event `json:"event"`
		}
		if err := wsjson.Read(ctx, conn, &frame); err != nil {
			t.Fatal(err)
		}
		if frame.Type == "permission_response_recorded" {
			ack = true
		}
		if frame.Event.Type == "prompt_completed" {
			completed = true
		}
	}
	if service.Status().State != "ready" {
		t.Fatal("prompt did not complete")
	}
}
