package httpapi

import (
	"log/slog"
	"net/http"

	wire "github.com/josemiguelmelo/orkest/internal/contracts/agentctl/v1"

	"github.com/josemiguelmelo/orkest/internal/adapters/http/agentctl/protocol"
	"github.com/josemiguelmelo/orkest/internal/platform/httplog"
)

func (s *Server) configure(w http.ResponseWriter, r *http.Request) {
	var request wire.RunConfig
	if !protocol.ReadJSON(w, r, &request) {
		return
	}
	if err := s.controller.Configure(fromWireRunConfig(request)); err != nil {
		writeServiceError(w, err, http.StatusInternalServerError, "internal_error", "internal service error")
		return
	}
	protocol.WriteJSON(w, http.StatusOK, map[string]string{"status": "configured"})
}

func (s *Server) launch(w http.ResponseWriter, r *http.Request) {
	requestLogger(w).Info("agent launch request received")
	if err := s.controller.Launch(); err != nil {
		requestLogger(w).Error("agent launch request failed", "reason", "runtime_launch_failed")
		writeServiceError(w, err, http.StatusBadGateway, "launch_failed", "provider startup failed")
		return
	}
	requestLogger(w).Info("agent launch request completed")
	protocol.WriteJSON(w, http.StatusOK, s.snapshot())
}

func requestLogger(w http.ResponseWriter) *slog.Logger {
	if logger := httplog.LoggerFromWriter(w); logger != nil {
		return logger
	}
	return slog.Default()
}

func (s *Server) restore(w http.ResponseWriter, r *http.Request) {
	if err := s.controller.RestoreSession(); err != nil {
		writeServiceError(w, err, http.StatusBadGateway, "launch_failed", "session restoration failed")
		return
	}
	protocol.WriteJSON(w, http.StatusOK, s.snapshot())
}
