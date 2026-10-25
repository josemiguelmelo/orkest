// Package httpapi composes the independent REST and WebSocket transport modules.
package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/agentctl"
	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi/rest"
	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi/websocket"
)

type Server struct {
	rest      *rest.Server
	websocket *websocket.Server
}

// NewServer binds both transports to the same controller and startup timestamp.
// The context owns stream connections, independently of accepted execution.
func NewServer(ctx context.Context, controller *agentctl.RunController) (*Server, error) {
	startedAt := time.Now().UTC()
	restServer, err := rest.NewServer(controller, startedAt)
	if err != nil {
		return nil, err
	}
	streamServer, err := websocket.NewServer(ctx, controller, startedAt)
	if err != nil {
		return nil, err
	}
	return &Server{rest: restServer, websocket: streamServer}, nil
}

func (s *Server) Handler() http.Handler {
	router := chi.NewRouter()
	s.rest.RegisterRoutes(router)
	s.websocket.RegisterRoutes(router)
	return router
}
