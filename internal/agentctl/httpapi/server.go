package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

// Server exposes the service through HTTP and WebSocket transports.
// Its context owns connections, independently of future agent execution.
type Server struct {
	ctx       context.Context
	startedAt time.Time
	service   *agentctl.Service
}

func NewServer(ctx context.Context, service *agentctl.Service) (*Server, error) {
	if service == nil {
		return nil, errors.New("agentctl service is required")
	}
	return &Server{ctx: ctx, service: service, startedAt: time.Now().UTC()}, nil
}

func (s *Server) Handler() http.Handler {
	mux := chi.NewRouter()
	mux.Get("/health", s.health)
	mux.Get("/info", s.info)
	mux.Post("/agent/configure", s.configure)
	mux.Post("/agent/launch", s.launch)
	mux.Get("/agent/stream", s.stream)
	return mux
}
