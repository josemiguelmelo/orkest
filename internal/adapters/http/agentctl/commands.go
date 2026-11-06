package httpapi

import (
	"net/http"

	wire "github.com/josemiguelmelo/orkest/internal/contracts/agentctl/v1"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/adapters/http/agentctl/protocol"
)

func (s *Server) prompt(w http.ResponseWriter, r *http.Request) {
	var request wire.PromptRequest
	if !protocol.ReadJSON(w, r, &request) {
		return
	}
	if err := s.controller.Prompt(request.CommandID, request.Text); err != nil {
		writeServiceError(w, err, http.StatusInternalServerError, "internal_error", "internal service error")
		return
	}
	protocol.WriteJSON(w, http.StatusAccepted, wire.CommandResponse{Status: "accepted", CommandID: request.CommandID})
}

func (s *Server) cancelPrompt(w http.ResponseWriter, r *http.Request) {
	var request wire.CancelRequest
	if !protocol.ReadJSON(w, r, &request) {
		return
	}
	if err := s.controller.CancelPrompt(request.CommandID); err != nil {
		writeServiceError(w, err, http.StatusInternalServerError, "internal_error", "internal service error")
		return
	}
	protocol.WriteJSON(w, http.StatusAccepted, wire.CommandResponse{Status: "cancel_requested", CommandID: request.CommandID})
}

func (s *Server) respondPermission(w http.ResponseWriter, r *http.Request) {
	var request wire.PermissionResponse
	if !protocol.ReadJSON(w, r, &request) {
		return
	}
	if err := s.controller.RespondPermission(fromWirePermissionResponse(request)); err != nil {
		writeServiceError(w, err, http.StatusInternalServerError, "internal_error", "internal service error")
		return
	}
	protocol.WriteJSON(w, http.StatusOK, wire.CommandResponse{Status: "permission_response_recorded", RequestID: request.RequestID})
}

func (s *Server) command(w http.ResponseWriter, r *http.Request) {
	status, err := s.controller.CommandStatus(chi.URLParam(r, "commandID"))
	if err != nil {
		writeServiceError(w, err, http.StatusNotFound, "invalid_command", "command not found")
		return
	}
	protocol.WriteJSON(w, http.StatusOK, wire.CommandStatus{CommandID: status.CommandID, Status: string(status.Status)})
}
