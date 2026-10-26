// Package httpapi composes the independent REST and WebSocket transport modules.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/agentctl"
	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi/rest"
	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi/websocket"
	"github.com/josemiguelmelo/orkest/internal/core/httplog"
	"github.com/josemiguelmelo/orkest/internal/core/openapi"
)

type Server struct {
	rest      *rest.Server
	websocket *websocket.Server
}

// NewServer binds both transports to the same controller and startup timestamp.
// The context owns stream connections, independently of accepted execution.
func NewServer(ctx context.Context, controller *agentctl.RunController) (*Server, error) {
	if controller == nil {
		return nil, errors.New("agentctl controller is required")
	}
	if _, err := openapi.LoadAndValidateOpenAPIDoc(OpenAPIContract, ctx); err != nil {
		return nil, err
	}
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

func (s *Server) Handler(loggers ...*slog.Logger) http.Handler {
	router := chi.NewRouter()
	var logger *slog.Logger
	if len(loggers) > 0 {
		logger = loggers[0]
	}
	router.Use(httplog.Middleware(logger))
	s.rest.RegisterRoutes(router)
	s.websocket.RegisterRoutes(router)
	openapi.Enable(router, "Orkest agentctl API", OpenAPIContract)
	return router
}
