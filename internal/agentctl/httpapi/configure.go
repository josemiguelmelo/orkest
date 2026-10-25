package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

func (s *Server) configure(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(w, 415, "application/json required")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read configuration body")
		return
	}
	var config agentctl.Configuration
	if err := json.Unmarshal(body, &config); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be a valid JSON configuration object")
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
