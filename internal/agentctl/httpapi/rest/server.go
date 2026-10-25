// Package rest serves agentctl commands and current status over HTTP.
package rest

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/agentctl"
	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi/protocol"
)

// Controller describes only the operations consumed by this transport.
type Controller interface {
	Status() agentctl.Status
	Configure(agentctl.RunConfig) error
	Launch() error
	Prompt(string, string) error
	CancelPrompt(string) error
	RespondPermission(agentctl.PermissionResponse) error
}

type Server struct {
	controller Controller
	startedAt  time.Time
}

func NewServer(controller Controller, startedAt time.Time) (*Server, error) {
	if controller == nil {
		return nil, errors.New("agentctl controller is required")
	}
	return &Server{controller: controller, startedAt: startedAt}, nil
}

func (s *Server) RegisterRoutes(router chi.Router) {
	router.Get("/health", s.health)
	router.Get("/info", s.info)
	router.Post("/agent/configure", s.configure)
	router.Post("/agent/launch", s.launch)
	router.Post("/agent/prompt", s.prompt)
	router.Post("/agent/cancel", s.cancelPrompt)
	router.Post("/agent/permissions/respond", s.respondPermission)
}

func (s *Server) Handler() http.Handler {
	router := chi.NewRouter()
	s.RegisterRoutes(router)
	return router
}

func (s *Server) snapshot() protocol.Info {
	return protocol.Snapshot(s.controller.Status(), s.startedAt)
}

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	protocol.WriteJSON(w, http.StatusOK, s.snapshot())
}
