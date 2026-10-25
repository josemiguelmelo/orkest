package rest

import (
	"net/http"

	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi/protocol"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

func (s *Server) configure(w http.ResponseWriter, r *http.Request) {
	var config agentctl.RunConfig
	if !protocol.ReadJSON(w, r, &config) {
		return
	}
	if err := s.controller.Configure(config); err != nil {
		writeServiceError(w, err, http.StatusInternalServerError, "internal_error", "internal service error")
		return
	}
	protocol.WriteJSON(w, http.StatusOK, map[string]string{"status": "configured"})
}
