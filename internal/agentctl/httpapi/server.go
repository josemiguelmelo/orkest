// Package httpapi composes the independent REST and WebSocket transport modules.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	contract "github.com/josemiguelmelo/orkest/api"
	"github.com/josemiguelmelo/orkest/internal/agentctl"
	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi/rest"
	"github.com/josemiguelmelo/orkest/internal/agentctl/httpapi/websocket"
	"github.com/swaggest/swgui/v5emb"
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
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromData(contract.AgentctlOpenAPI)
	if err != nil {
		return nil, fmt.Errorf("load agentctl OpenAPI contract: %w", err)
	}
	if err := doc.Validate(ctx); err != nil {
		return nil, fmt.Errorf("validate agentctl OpenAPI contract: %w", err)
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

func (s *Server) Handler() http.Handler {
	router := chi.NewRouter()
	s.rest.RegisterRoutes(router)
	s.websocket.RegisterRoutes(router)
	router.Get("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write(contract.AgentctlOpenAPI)
	})
	router.Handle("/docs/*", v5emb.New("Orkest agentctl API", "/openapi.yaml", "/docs/"))
	router.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/", http.StatusPermanentRedirect)
	})
	return router
}
