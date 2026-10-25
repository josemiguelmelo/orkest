package rest

import (
	"errors"
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
		if errors.Is(err, agentctl.ErrConfigurationConflict) {
			protocol.WriteError(w, http.StatusConflict, err.Error())
		} else {
			protocol.WriteError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	protocol.WriteJSON(w, 200, map[string]string{"status": "configured"})
}
