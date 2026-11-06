package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	wire "github.com/josemiguelmelo/orkest/internal/contracts/agentctl/v1"

	ws "github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/josemiguelmelo/orkest/internal/adapters/http/agentctl/protocol"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentrun "github.com/josemiguelmelo/orkest/internal/agentctl/run"
	"github.com/josemiguelmelo/orkest/internal/platform/httplog"
)

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	logger := httplog.LoggerFromWriter(w)
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("agent activity stream request received")
	if !protocol.Snapshot(s.controller.Status(), s.startedAt).Configured {
		logger.Warn("agent activity stream rejected", "reason", "run_not_configured")
		protocol.WriteError(w, http.StatusConflict, "not_configured", "configure the agent before connecting")
		return
	}
	after, err := strconv.ParseUint(r.URL.Query().Get("afterSequence"), 10, 64)
	if r.URL.Query().Get("afterSequence") == "" {
		after = 0
		err = nil
	}
	if err != nil {
		logger.Warn("agent activity stream rejected", "reason", "invalid_cursor")
		protocol.WriteValidationError(w, http.StatusBadRequest, "invalid_cursor", &protocol.ValidationError{
			Field:   "afterSequence",
			Message: "invalid afterSequence",
		})
		return
	}
	replay, events, unsubscribe, err := s.controller.Subscribe(after)
	if err != nil {
		logger.Warn("agent activity stream subscription failed", "reason", "subscription_unavailable")
		if errors.Is(err, agentrun.ErrCursor) {
			protocol.WriteValidationError(w, http.StatusBadRequest, "invalid_cursor", &protocol.ValidationError{
				Field:   "afterSequence",
				Message: agentrun.ErrCursor.Error(),
			})
		} else if errors.Is(err, interaction.ErrConversation) {
			protocol.WriteError(w, http.StatusConflict, "conversation_unavailable", "conversation unavailable")
		} else {
			protocol.WriteError(w, http.StatusInternalServerError, "internal_error", "internal service error")
		}
		return
	}
	defer unsubscribe()
	conn, err := ws.Accept(w, r, nil)
	if err != nil {
		logger.Warn("agent activity stream websocket upgrade failed")
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
	if err := send(ctx, conn, wire.StreamFrame{Type: "ready", Info: &info}); err != nil {
		logger.Warn("agent activity stream ready frame failed")
		return
	}
	logger.Info("agent activity stream ready", "replay_count", len(replay))
	for _, event := range replay {
		if err := send(ctx, conn, wire.StreamFrame{Type: "event", Event: eventPointer(event)}); err != nil {
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
			if err := send(ctx, conn, wire.StreamFrame{Type: "event", Event: eventPointer(event)}); err != nil {
				return
			}
		}
	}
}

func send(ctx context.Context, conn *ws.Conn, value wire.StreamFrame) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return wsjson.Write(ctx, conn, value)
}

func eventPointer(event interaction.Event) *wire.Event { mapped := toWireEvent(event); return &mapped }
