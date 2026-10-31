package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/josemiguelmelo/orkest/internal/agent"
	"github.com/josemiguelmelo/orkest/internal/coderepository"
	"github.com/josemiguelmelo/orkest/internal/core/openapi"
	"github.com/josemiguelmelo/orkest/internal/executionprofile"
	"github.com/josemiguelmelo/orkest/internal/identity"
	"github.com/josemiguelmelo/orkest/internal/mcpserver"
	"github.com/josemiguelmelo/orkest/internal/orchestration"
	"github.com/josemiguelmelo/orkest/internal/organization"
	"github.com/josemiguelmelo/orkest/internal/skill"
	"github.com/josemiguelmelo/orkest/internal/task"
	"github.com/josemiguelmelo/orkest/internal/taskintegration"
	"github.com/josemiguelmelo/orkest/internal/workspace"
	"github.com/josemiguelmelo/orkest/internal/workspaceenv"
)

// NewRouter validates the served OpenAPI contract before exposing routes.
func NewRouter(users *identity.UserRegistrationService, organizations *organization.OrganizationService, workspaces *workspace.WorkspaceService, agents *agent.Service, codeRepositories *coderepository.Service, tasks *task.Service, integrations *taskintegration.TaskIntegrationService, mcpServers *mcpserver.MCPServerService, skillCatalog *skill.CatalogService, launches *orchestration.LaunchService, profiles *executionprofile.ExecutionProfileService, environment ...*workspaceenv.Service) (*chi.Mux, error) {
	if profiles == nil {
		return nil, fmt.Errorf("execution profile service is required")
	}
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
			executionProfileAPI{service: profiles}.routes(api)
			if skillCatalog != nil {
				skillAPI{service: skillCatalog, scope: skill.ScopeGlobal}.routes(api, "/skills")
				skillAPI{service: skillCatalog, scope: skill.ScopeOrganization}.routes(api, "/organizations/{orgID}/skills")
				skillAPI{service: skillCatalog, scope: skill.ScopeWorkspace}.routes(api, "/organizations/{orgID}/workspaces/{workspaceID}/skills")
			}
			if mcpServers != nil {
				mcpServerAPI{service: mcpServers}.routes(api)
			}
			if len(environment) > 0 && environment[0] != nil {
				workspaceEnvironmentAPI{service: environment[0]}.routes(api)
			}
			codeRepositoryAPI{repositories: codeRepositories}.routes(api)
			codingIntegrationAPI{service: codeRepositories}.routes(api)
			taskAPI{tasks: tasks}.routes(api)
			if launches != nil {
				taskRunAPI{tasks: tasks, launches: launches}.routes(api)
			}
			if integrations != nil {
				taskIntegrationAPI{service: integrations}.routes(api)
			}
		})
	})
	openapi.Enable(router, "Orkest API", OpenAPIContract)
	mountWeb(router, os.Getenv("ORKEST_WEB_DIR"))
	return router, nil
}
