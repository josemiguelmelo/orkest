package websocket

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi/protocol"

	"github.com/josemiguelmelo/orkest/internal/agentctl"

	ws "github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// streamMessage contains only server-originated readiness and activity frames.
type streamMessage struct {
	Type  string          `json:"type"`
	Info  *protocol.Info  `json:"info,omitempty"`
	Event *agentctl.Event `json:"event,omitempty"`
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	if !protocol.Snapshot(s.controller.Status(), s.startedAt).Configured {
		protocol.WriteError(w, 409, "configure the agent before connecting")
		return
	}
	after, err := strconv.ParseUint(r.URL.Query().Get("afterSequence"), 10, 64)
	if r.URL.Query().Get("afterSequence") == "" {
		after = 0
		err = nil
	}
	if err != nil {
		protocol.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid afterSequence", "errorCode": "invalid_cursor"})
		return
	}
	replay, events, unsubscribe, err := s.controller.Subscribe(after)
	if err != nil {
		if errors.Is(err, agentctl.ErrCursor) {
			protocol.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error(), "errorCode": "invalid_cursor"})
		} else {
			protocol.WriteError(w, http.StatusConflict, "conversation unavailable")
		}
		return
	}
	defer unsubscribe()
	conn, err := ws.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer func(conn *ws.Conn) { _ = conn.CloseNow() }(conn)
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	// Shutdown must also close hijacked HTTP connections.
	stop := context.AfterFunc(ctx, func() { _ = conn.CloseNow() })
	defer stop()
	// CloseRead handles control frames and disconnects, and rejects all client
	// application messages. The stream never accepts execution commands.
	ctx = conn.CloseRead(ctx)
	info := protocol.Snapshot(s.controller.Status(), s.startedAt)
	if err := send(ctx, conn, streamMessage{Type: "ready", Info: &info}); err != nil {
		return
	}
	for _, event := range replay {
		if err := send(ctx, conn, streamMessage{Type: "event", Event: &event}); err != nil {
			return
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				_ = conn.Close(ws.StatusPolicyViolation, "subscriber fell behind; reconnect with afterSequence")
				return
			}
			if err := send(ctx, conn, streamMessage{Type: "event", Event: &event}); err != nil {
				return
			}
		}
	}
}

func send(ctx context.Context, conn *ws.Conn, value streamMessage) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return wsjson.Write(ctx, conn, value)
}
