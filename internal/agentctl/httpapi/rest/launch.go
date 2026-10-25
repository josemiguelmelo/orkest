package rest

import (
	"net/http"

	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi/protocol"
)

func (s *Server) launch(w http.ResponseWriter, r *http.Request) {
	if err := s.controller.Launch(); err != nil {
		writeServiceError(w, err, http.StatusBadGateway, "launch_failed", "provider startup failed")
		return
	}
	protocol.WriteJSON(w, http.StatusOK, s.snapshot())
}
