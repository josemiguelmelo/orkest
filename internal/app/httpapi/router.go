package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/agent"
	"github.com/josemiguelmelo/orkest/internal/coderepository"
	"github.com/josemiguelmelo/orkest/internal/core/openapi"
	"github.com/josemiguelmelo/orkest/internal/identity"
	"github.com/josemiguelmelo/orkest/internal/orchestration"
	"github.com/josemiguelmelo/orkest/internal/organization"
	"github.com/josemiguelmelo/orkest/internal/task"
	"github.com/josemiguelmelo/orkest/internal/taskintegration"
	"github.com/josemiguelmelo/orkest/internal/workspace"
)

// NewRouter validates the served OpenAPI contract before exposing routes.
func NewRouter(users *identity.UserRegistrationService, organizations *organization.OrganizationService, workspaces *workspace.WorkspaceService, agents *agent.Service, codeRepositories *coderepository.Service, tasks *task.Service, integrations *taskintegration.TaskIntegrationService, tempLaunches ...*orchestration.LaunchService) (*chi.Mux, error) {
	if _, err := openapi.LoadAndValidateOpenAPIDoc(OpenAPIContract, context.Background()); err != nil {
		return nil, fmt.Errorf("load platform OpenAPI contract: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	auth, err := newOIDCAuth(ctx, users, organizations)
	if err != nil {
		return nil, err
	}

	router := chi.NewRouter()
	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	router.Get("/auth/login", auth.login)
	router.Get("/auth/callback", auth.callback)
	router.Post("/auth/logout", auth.logout)
	router.Group(func(api chi.Router) {
		api.Use(auth.middleware)
		api.Route("/api", func(api chi.Router) {
			api.Get("/me", auth.currentUser)
			organizationAPI{organizations: organizations, workspaces: workspaces}.routes(api)
			agentAPI{agents: agents}.routes(api)
			codeRepositoryAPI{repositories: codeRepositories}.routes(api)
			taskAPI{tasks: tasks}.routes(api)
			if integrations != nil {
				taskIntegrationAPI{service: integrations}.routes(api)
			}
			if len(tempLaunches) > 0 && tempLaunches[0] != nil {
				tempAgentLaunchAPI{launches: tempLaunches[0]}.routes(api)
			}
		})
	})
	openapi.Enable(router, "Orkest API", OpenAPIContract)
	return router, nil
}
