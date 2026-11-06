package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	wire "github.com/josemiguelmelo/orkest/internal/contracts/agentctl/v1"

	ws "github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentrun "github.com/josemiguelmelo/orkest/internal/agentctl/run"
)

func TestStream(t *testing.T) {
	s, server, cancel := setupStreamServer(t)
	configure(t, s)
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	conn, _, err := ws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/agent/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func(conn *ws.Conn) { _ = conn.CloseNow() }(conn)
	var event wire.StreamFrame
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
			s, server, _ := setupStreamServer(t)
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

func setupStreamServer(t *testing.T) (*Server, *httptest.Server, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	server, err := NewServer(ctx, &streamController{})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler(nil))
	t.Cleanup(func() { cancel(); httpServer.Close() })
	return server, httpServer, cancel
}

type streamController struct {
	commandController
	configured     bool
	subscribeError error
}

func (c *streamController) Status() agentrun.RunStatus {
	return agentrun.RunStatus{Configured: c.configured}
}

func (c *streamController) Subscribe(uint64) ([]interaction.Event, <-chan interaction.Event, func(), error) {
	return nil, make(chan interaction.Event), func() {}, c.subscribeError
}

func configure(t *testing.T, server *Server) {
	t.Helper()
	server.controller.(*streamController).configured = true
}

func TestStreamValidationErrors(t *testing.T) {
	for _, tc := range []struct {
		name, query, code string
		configured        bool
		err               error
		status            int
	}{
		{"unconfigured", "", "not_configured", false, nil, 409},
		{"malformed cursor", "?afterSequence=no", "invalid_cursor", true, nil, 400},
		{"expired cursor", "?afterSequence=1", "invalid_cursor", true, agentrun.ErrCursor, 400},
		{"unavailable", "", "conversation_unavailable", true, interaction.ErrConversation, 409},
		{"unexpected", "", "internal_error", true, errors.New("SECRET"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, err := NewServer(context.Background(), &streamController{configured: tc.configured, subscribeError: tc.err})
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			server.Handler(nil).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/agent/stream"+tc.query, nil))
			var body wire.Error
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != tc.status || body.Code != tc.code || body.Message == "" {
				t.Fatalf("unexpected error: %d %+v", response.Code, body)
			}
			if strings.Contains(response.Body.String(), "SECRET") {
				t.Fatal("error leaked internal details")
			}
		})
	}
}
