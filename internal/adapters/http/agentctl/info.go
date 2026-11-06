package httpapi

import (
	"net/http"

	wire "github.com/josemiguelmelo/orkest/internal/contracts/agentctl/v1"

	"github.com/josemiguelmelo/orkest/internal/adapters/http/agentctl/protocol"
)

func (s *Server) snapshot() wire.Info {
	return protocol.Snapshot(s.controller.Status(), s.startedAt)
}

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	protocol.WriteJSON(w, http.StatusOK, s.snapshot())
}
