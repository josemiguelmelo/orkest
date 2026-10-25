package rest

import (
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
		writeServiceError(w, err, http.StatusInternalServerError, "internal_error", "internal service error")
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
		writeServiceError(w, err, http.StatusInternalServerError, "internal_error", "internal service error")
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
		writeServiceError(w, err, http.StatusInternalServerError, "internal_error", "internal service error")
		return
	}
	protocol.WriteJSON(w, http.StatusOK, commandResponse{Status: "permission_response_recorded", RequestID: request.RequestID})
}
