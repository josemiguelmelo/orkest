package httpapi

import (
	"errors"
	"net/http"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

func (s *Server) configure(w http.ResponseWriter, r *http.Request) {
	var config agentctl.Configuration
	if !readJSON(w, r, &config) {
		return
	}
	if err := s.service.Configure(config); err != nil {
		if errors.Is(err, agentctl.ErrConfigurationConflict) {
			writeError(w, http.StatusConflict, err.Error())
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeJSON(w, 200, map[string]string{"status": "configured"})
}
