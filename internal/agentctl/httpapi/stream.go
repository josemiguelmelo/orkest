package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type message struct {
	Type       string                       `json:"type"`
	RequestID  string                       `json:"requestId,omitempty"`
	Info       *Info                        `json:"info,omitempty"`
	Error      string                       `json:"error,omitempty"`
	Text       string                       `json:"text,omitempty"`
	CommandID  string                       `json:"commandId,omitempty"`
	Permission *agentctl.PermissionResponse `json:"permission,omitempty"`
	Event      *agentctl.Event              `json:"event,omitempty"`
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	if !s.snapshot().Configured {
		writeError(w, 409, "configure the agent before connecting")
		return
	}
	after, err := strconv.ParseUint(r.URL.Query().Get("afterSequence"), 10, 64)
	if r.URL.Query().Get("afterSequence") == "" {
		after = 0
		err = nil
	}
	if err != nil {
		writeError(w, 400, "invalid afterSequence")
		return
	}
	replay, events, unsubscribe, err := s.service.Subscribe(after)
	if err != nil {
		writeError(w, 400, "invalid afterSequence")
		return
	}
	defer unsubscribe()
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
	for _, event := range replay {
		if err := send(ctx, conn, message{Type: "event", Event: &event}); err != nil {
			return
		}
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-events:
				if !ok {
					_ = conn.Close(websocket.StatusPolicyViolation, "subscriber fell behind; reconnect with afterSequence")
					return
				}
				if err := send(ctx, conn, message{Type: "event", Event: &event}); err != nil {
					_ = conn.CloseNow()
					return
				}
			}
		}
	}()
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
		case "prompt":
			if err := s.service.Prompt(request.RequestID, request.Text); err != nil {
				response.Error = err.Error()
			} else {
				response = message{Type: "accepted", RequestID: request.RequestID}
			}
		case "cancel":
			if err := s.service.CancelPrompt(request.CommandID); err != nil {
				response.Error = err.Error()
			} else {
				response = message{Type: "cancel_requested", RequestID: request.RequestID}
			}
		case "permission_response":
			if request.Permission == nil {
				response.Error = "permission response required"
			} else if err := s.service.RespondPermission(*request.Permission); err != nil {
				response.Error = err.Error()
			} else {
				response = message{Type: "permission_response_recorded", RequestID: request.RequestID}
			}

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
