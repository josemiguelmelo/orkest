package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type message struct {
	Type      string `json:"type"`
	RequestID string `json:"requestId,omitempty"`
	Info      *Info  `json:"info,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	if !s.snapshot().Configured {
		writeError(w, 409, "configure the agent before connecting")
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func(conn *websocket.Conn) { _ = conn.CloseNow() }(conn)
	conn.SetReadLimit(-1)
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	// Shutdown must also close hijacked HTTP connections.
	stop := context.AfterFunc(ctx, func() { _ = conn.CloseNow() })
	defer stop()
	info := s.snapshot()
	if err := send(ctx, conn, message{Type: "ready", Info: &info}); err != nil {
		return
	}
	for {
		var request message
		kind, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if kind != websocket.MessageText {
			_ = conn.Close(websocket.StatusUnsupportedData, "text JSON required")
			return
		}
		if err := json.Unmarshal(data, &request); err != nil {
			_ = conn.Close(websocket.StatusInvalidFramePayloadData, "invalid JSON")
			return
		}
		response := message{Type: "error", RequestID: request.RequestID, Error: "unsupported_command"}
		switch request.Type {
		case "ping":
			response = message{Type: "pong", RequestID: request.RequestID}
		case "info":
			info := s.snapshot()
			response = message{Type: "info", RequestID: request.RequestID, Info: &info}
		}
		if err := send(ctx, conn, response); err != nil {
			return
		}
	}
}

func send(ctx context.Context, conn *websocket.Conn, value message) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return wsjson.Write(ctx, conn, value)
}
