// Package httpapi serves agentctl commands, status, and receive-only activity.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentrun "github.com/josemiguelmelo/orkest/internal/agentctl/run"
	"github.com/josemiguelmelo/orkest/internal/platform/httplog"
	"github.com/josemiguelmelo/orkest/internal/platform/openapi"
)

// Controller describes the execution operations consumed by the HTTP API.
type Controller interface {
	Status() agentrun.RunStatus
	Configure(runconfig.RunConfig) error
	Launch() error
	RestoreSession() error
	CommandStatus(string) (interaction.CommandStatus, error)
	ResumeQueue() error
	DiscardQueue() error
	Prompt(string, string) error
	CancelPrompt(string) error
	RespondPermission(interaction.PermissionResponse) error
	Subscribe(uint64) ([]interaction.Event, <-chan interaction.Event, func(), error)
}

type Server struct {
	ctx        context.Context
	controller Controller
	startedAt  time.Time
}

// NewServer binds command and stream handlers to the same controller. The
// context owns stream connections independently of accepted execution.
func NewServer(ctx context.Context, controller Controller) (*Server, error) {
	if controller == nil {
		return nil, errors.New("agentctl controller is required")
	}
	if _, err := openapi.LoadAndValidateOpenAPIDoc(OpenAPIContract, ctx); err != nil {
		return nil, err
	}
	return &Server{ctx: ctx, controller: controller, startedAt: time.Now().UTC()}, nil
}

func (s *Server) Handler(logger *slog.Logger) http.Handler {
	router := chi.NewRouter()
	router.Use(httplog.Middleware(logger))
	router.Get("/health", s.health)
	router.Get("/info", s.info)
	router.Post("/agent/configure", s.configure)
	router.Post("/agent/launch", s.launch)
	router.Post("/agent/restore", s.restore)
	router.Get("/agent/commands/{commandID}", s.command)
	router.Post("/agent/queue/resume", s.resumeQueue)
	router.Post("/agent/queue/discard", s.discardQueue)
	router.Post("/agent/prompt", s.prompt)
	router.Post("/agent/cancel", s.cancelPrompt)
	router.Post("/agent/permissions/respond", s.respondPermission)
	router.Get("/agent/stream", s.stream)
	openapi.Enable(router, "Orkest agentctl API", OpenAPIContract)
	return router
}
