package httpapi

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestStream(t *testing.T) {
	s, server, cancel := setup(t)
	request(s, "POST", "/agent/configure", validConfig)
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/agent/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	var event message
	if err := wsjson.Read(ctx, conn, &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "ready" || event.Info == nil || !event.Info.Configured {
		t.Fatalf("unexpected ready: %+v", event)
	}
	// Large messages and extra fields follow the same permissive decoding as HTTP.
	data := `{"type":"ping","requestId":"large","extra":"` + strings.Repeat("x", 128<<10) + `"}`
	if err := conn.Write(ctx, websocket.MessageText, []byte(data)); err != nil {
		t.Fatal(err)
	}
	if err := wsjson.Read(ctx, conn, &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "pong" || event.RequestID != "large" {
		t.Fatalf("unexpected response to large message: %+v", event)
	}
	for _, tc := range []struct{ command, response string }{{"ping", "pong"}, {"info", "info"}, {"prompt", "error"}, {"approve", "error"}} {
		if err := wsjson.Write(ctx, conn, message{Type: tc.command, RequestID: "req-1"}); err != nil {
			t.Fatal(err)
		}
		event = message{}
		if err := wsjson.Read(ctx, conn, &event); err != nil {
			t.Fatal(err)
		}
		if event.Type != tc.response || event.RequestID != "req-1" {
			t.Fatalf("unexpected response %+v", event)
		}
	}
	_ = conn.CloseNow()
	if !s.snapshot().Configured {
		t.Fatal("disconnect cleared configuration")
	}
	conn, _, err = websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/agent/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
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
		kind   websocket.MessageType
		data   string
		status websocket.StatusCode
	}{
		{"binary", websocket.MessageBinary, `{}`, websocket.StatusUnsupportedData},
		{"invalid JSON", websocket.MessageText, `{`, websocket.StatusInvalidFramePayloadData},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, server, _ := setup(t)
			request(s, "POST", "/agent/configure", validConfig)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/agent/stream", nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.CloseNow()
			if _, _, err := conn.Read(ctx); err != nil {
				t.Fatal(err)
			}
			if err := conn.Write(ctx, tc.kind, []byte(tc.data)); err != nil {
				t.Fatal(err)
			}
			_, _, err = conn.Read(ctx)
			if websocket.CloseStatus(err) != tc.status {
				t.Fatalf("unexpected close: %v", err)
			}
		})
	}
}
