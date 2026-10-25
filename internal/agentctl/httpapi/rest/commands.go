package rest

import (
	"errors"
	"net/http"

	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi/protocol"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

type promptRequest struct {
	CommandID string `json:"commandId"`
	Text      string `json:"text"`
}

type cancelRequest struct {
	CommandID string `json:"commandId"`
}

type commandResponse struct {
	Status    string `json:"status"`
	CommandID string `json:"commandId,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

func (s *Server) prompt(w http.ResponseWriter, r *http.Request) {
	var request promptRequest
	if !protocol.ReadJSON(w, r, &request) {
		return
	}
	if err := s.controller.Prompt(request.CommandID, request.Text); err != nil {
		writeCommandError(w, err)
		return
	}
	protocol.WriteJSON(w, http.StatusAccepted, commandResponse{Status: "accepted", CommandID: request.CommandID})
}

func (s *Server) cancelPrompt(w http.ResponseWriter, r *http.Request) {
	var request cancelRequest
	if !protocol.ReadJSON(w, r, &request) {
		return
	}
	if err := s.controller.CancelPrompt(request.CommandID); err != nil {
		writeCommandError(w, err)
		return
	}
	protocol.WriteJSON(w, http.StatusAccepted, commandResponse{Status: "cancel_requested", CommandID: request.CommandID})
}

func (s *Server) respondPermission(w http.ResponseWriter, r *http.Request) {
	var request agentctl.PermissionResponse
	if !protocol.ReadJSON(w, r, &request) {
		return
	}
	if err := s.controller.RespondPermission(request); err != nil {
		writeCommandError(w, err)
		return
	}
	protocol.WriteJSON(w, http.StatusOK, commandResponse{Status: "permission_response_recorded", RequestID: request.RequestID})
}

func writeCommandError(w http.ResponseWriter, err error) {
	status, code, message := commandError(err)
	protocol.WriteJSON(w, status, map[string]string{"errorCode": code, "error": message})
}

func commandError(err error) (int, string, string) {
	switch {
	case errors.Is(err, agentctl.ErrCommand):
		return http.StatusBadRequest, "invalid_command", agentctl.ErrCommand.Error()
	case errors.Is(err, agentctl.ErrPermission):
		return http.StatusConflict, "invalid_permission", agentctl.ErrPermission.Error()
	case errors.Is(err, agentctl.ErrConversation):
		return http.StatusConflict, "conversation_unavailable", agentctl.ErrConversation.Error()
	case errors.Is(err, agentctl.ErrCommandLimit):
		return http.StatusConflict, "command_limit", agentctl.ErrCommandLimit.Error()
	case errors.Is(err, agentctl.ErrCancellation):
		return http.StatusBadGateway, "cancellation_failed", agentctl.ErrCancellation.Error()
	default:
		return http.StatusServiceUnavailable, "storage_unavailable", "conversation storage unavailable; reconciliation required"
	}
}
