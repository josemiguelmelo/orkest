package httpapi

import (
	"net/http"

	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi/protocol"
)

func (s *Server) snapshot() protocol.Info {
	return protocol.Snapshot(s.controller.Status(), s.startedAt)
}

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	protocol.WriteJSON(w, http.StatusOK, s.snapshot())
}
