package rest

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/agentctl"
	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi/protocol"
)

func (s *Server) restore(w http.ResponseWriter, r *http.Request) {
	controller, ok := s.controller.(interface{ Restore() error })
	if !ok {
		protocol.WriteError(w, http.StatusServiceUnavailable, "provider_unavailable", "session restoration unavailable")
		return
	}
	if err := controller.Restore(); err != nil {
		writeServiceError(w, err, http.StatusBadGateway, "launch_failed", "session restoration failed")
		return
	}
	protocol.WriteJSON(w, http.StatusOK, s.snapshot())
}

func (s *Server) command(w http.ResponseWriter, r *http.Request) {
	controller, ok := s.controller.(interface {
		Command(string) (agentctl.CommandStatus, error)
	})
	if !ok {
		protocol.WriteError(w, http.StatusServiceUnavailable, "provider_unavailable", "command lookup unavailable")
		return
	}
	status, err := controller.Command(chi.URLParam(r, "commandID"))
	if err != nil {
		writeServiceError(w, err, http.StatusNotFound, "invalid_command", "command not found")
		return
	}
	protocol.WriteJSON(w, http.StatusOK, status)
}

func (s *Server) resumeQueue(w http.ResponseWriter, r *http.Request) { s.queue(w, r, false) }

func (s *Server) discardQueue(w http.ResponseWriter, r *http.Request) { s.queue(w, r, true) }

func (s *Server) queue(w http.ResponseWriter, _ *http.Request, discard bool) {
	controller, ok := s.controller.(interface{ Queue(bool) error })
	if !ok {
		protocol.WriteError(w, http.StatusServiceUnavailable, "provider_unavailable", "queue control unavailable")
		return
	}
	if err := controller.Queue(discard); err != nil {
		writeServiceError(w, err, http.StatusConflict, "conversation_unavailable", "queue unavailable")
		return
	}
	protocol.WriteJSON(w, http.StatusOK, s.snapshot())
}
