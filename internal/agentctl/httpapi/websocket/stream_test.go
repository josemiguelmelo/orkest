package websocket

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"

	ws "github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestStream(t *testing.T) {
	s, server, cancel := setup(t)
	configure(t, s)
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	conn, _, err := ws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/agent/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func(conn *ws.Conn) { _ = conn.CloseNow() }(conn)
	var event streamMessage
	if err := wsjson.Read(ctx, conn, &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "ready" || event.Info == nil || !event.Info.Configured {
		t.Fatalf("unexpected ready: %+v", event)
	}
	// WebSocket protocol ping/pong remains available without application commands.
	pingCtx := conn.CloseRead(ctx)
	if err := conn.Ping(pingCtx); err != nil {
		t.Fatal(err)
	}
	_ = conn.CloseNow()
	if !s.controller.Status().Configured {
		t.Fatal("disconnect cleared configuration")
	}
	conn, _, err = ws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/agent/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func(conn *ws.Conn) { _ = conn.CloseNow() }(conn)
	if err := wsjson.Read(ctx, conn, &event); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := wsjson.Read(ctx, conn, &event); err == nil {
		t.Fatal("stream survived server shutdown")
	}
}

func TestInvalidStreamMessages(t *testing.T) {
	for _, tc := range []struct {
		name   string
		kind   ws.MessageType
		data   string
		status ws.StatusCode
	}{
		{"prompt command", ws.MessageText, `{"type":"prompt","requestId":"first","text":"hello"}`, ws.StatusPolicyViolation},
		{"permission command", ws.MessageText, `{"type":"permission_response"}`, ws.StatusPolicyViolation},
		{"ping command", ws.MessageText, `{"type":"ping"}`, ws.StatusPolicyViolation},
		{"binary", ws.MessageBinary, `{}`, ws.StatusPolicyViolation},
		{"invalid JSON", ws.MessageText, `{`, ws.StatusPolicyViolation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, server, _ := setup(t)
			configure(t, s)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, _, err := ws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/agent/stream", nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func(conn *ws.Conn) { _ = conn.CloseNow() }(conn)
			if _, _, err := conn.Read(ctx); err != nil {
				t.Fatal(err)
			}
			if err := conn.Write(ctx, tc.kind, []byte(tc.data)); err != nil {
				t.Fatal(err)
			}
			_, _, err = conn.Read(ctx)
			if ws.CloseStatus(err) != tc.status {
				t.Fatalf("unexpected close: %v", err)
			}
		})
	}
}

func setup(t *testing.T) (*Server, *httptest.Server, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	server, err := NewServer(ctx, agentctl.NewRunController(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(func() { cancel(); httpServer.Close() })
	return server, httpServer, cancel
}

func configure(t *testing.T, server *Server) {
	t.Helper()
	if err := server.controller.Configure(agentctl.RunConfig{
		OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run",
		Runtime: "opencode", Model: agentctl.Model{Provider: "provider", ID: "model"}, SystemPrompt: "Work on this task",
	}); err != nil {
		t.Fatal(err)
	}
}
