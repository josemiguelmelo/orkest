package httpapi

import (
	"net/http"

	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi/protocol"
)

func (s *Server) resumeQueue(w http.ResponseWriter, _ *http.Request) {
	s.changeQueue(w, s.controller.ResumeQueue)
}

func (s *Server) discardQueue(w http.ResponseWriter, _ *http.Request) {
	s.changeQueue(w, s.controller.DiscardQueue)
}

func (s *Server) changeQueue(w http.ResponseWriter, change func() error) {
	if err := change(); err != nil {
		writeServiceError(w, err, http.StatusConflict, "conversation_unavailable", "queue unavailable")
		return
	}
	protocol.WriteJSON(w, http.StatusOK, s.snapshot())
}
