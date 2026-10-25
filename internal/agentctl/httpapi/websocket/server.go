// Package websocket serves agentctl readiness, replay, and live activity.
package websocket

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

type Server struct {
	ctx        context.Context
	controller *agentctl.RunController
	startedAt  time.Time
}

func NewServer(ctx context.Context, controller *agentctl.RunController, startedAt time.Time) (*Server, error) {
	if controller == nil {
		return nil, errors.New("agentctl controller is required")
	}
	return &Server{ctx: ctx, controller: controller, startedAt: startedAt}, nil
}

func (s *Server) RegisterRoutes(router chi.Router) {
	router.Get("/agent/stream", s.stream)
}

func (s *Server) Handler() http.Handler {
	router := chi.NewRouter()
	s.RegisterRoutes(router)
	return router
}
