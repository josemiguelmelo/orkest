package rest

import (
	"errors"
	"net/http"

	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi/protocol"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

func (s *Server) launch(w http.ResponseWriter, r *http.Request) {
	if err := s.controller.Launch(); err != nil {
		code := http.StatusBadGateway
		if errors.Is(err, agentctl.ErrLaunchConflict) || errors.Is(err, agentctl.ErrNotConfigured) {
			code = http.StatusConflict
		}
		if errors.Is(err, agentctl.ErrProviderUnavailable) {
			code = http.StatusServiceUnavailable
		}
		protocol.WriteError(w, code, err.Error())
		return
	}
	protocol.WriteJSON(w, http.StatusOK, s.snapshot())
}
