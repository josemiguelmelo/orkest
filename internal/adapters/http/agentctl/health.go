package httpapi

import (
	"net/http"

	"github.com/josemiguelmelo/orkest/internal/adapters/http/agentctl/protocol"
)

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	protocol.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
