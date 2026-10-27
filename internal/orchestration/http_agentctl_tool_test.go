package orchestration

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
)

func TestAgentctlStreamPreservesToolDetails(t *testing.T) {
	output := strings.Repeat("file contents\n", 3000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.CloseNow() }()
		if err = wsjson.Write(r.Context(), conn, map[string]any{"type": "ready"}); err != nil {
			t.Error(err)
			return
		}
		event := map[string]any{"type": "update", "sequence": 1, "runId": "run", "commandId": "command", "update": map[string]any{"kind": "tool_call_update", "toolCallId": "read-1", "title": "read", "toolKind": "read", "status": "completed", "input": `{"filePath":"README.md"}`, "output": output, "locations": []any{map[string]any{"path": "README.md", "line": 1}}, "content": []any{map[string]any{"kind": "text", "text": "file contents"}}}}
		if err = wsjson.Write(r.Context(), conn, map[string]any{"type": "event", "event": event}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := errors.New("event received")
	var received interaction.Event
	err := NewHTTPAgentctlClient(server.Client()).Connect(ctx, server.URL, 0, func() {}, func(event interaction.Event) error { received = event; return done })
	if !errors.Is(err, done) {
		t.Fatalf("stream failed: %v", err)
	}
	u := received.Update
	if u == nil || u.Input != `{"filePath":"README.md"}` || u.Output != output || u.ToolKind != "read" || len(u.Locations) != 1 || u.Locations[0].Path != "README.md" || len(u.Content) != 1 || u.Content[0].Text != "file contents" {
		t.Fatalf("stream dropped tool detail fields")
	}
}
