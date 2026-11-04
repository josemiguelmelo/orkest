package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/josemiguelmelo/orkest/internal/server/workflowautomation"

	"github.com/go-chi/chi/v5"
	appmcp "github.com/josemiguelmelo/orkest/internal/adapters/mcp"
	"github.com/josemiguelmelo/orkest/internal/platform/openapi"
	"github.com/josemiguelmelo/orkest/internal/server/agent"
	"github.com/josemiguelmelo/orkest/internal/server/agentruntimeprofile"
	"github.com/josemiguelmelo/orkest/internal/server/artifact"
	"github.com/josemiguelmelo/orkest/internal/server/coderepository"
	"github.com/josemiguelmelo/orkest/internal/server/executionprofile"
	"github.com/josemiguelmelo/orkest/internal/server/identity"
	"github.com/josemiguelmelo/orkest/internal/server/mcpserver"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
	"github.com/josemiguelmelo/orkest/internal/server/organization"
	"github.com/josemiguelmelo/orkest/internal/server/skill"
	"github.com/josemiguelmelo/orkest/internal/server/task"
	"github.com/josemiguelmelo/orkest/internal/server/taskintegration"
	"github.com/josemiguelmelo/orkest/internal/server/workflow"
	"github.com/josemiguelmelo/orkest/internal/server/workspace"
	"github.com/josemiguelmelo/orkest/internal/server/workspaceenv"
)

// NewRouter validates the served OpenAPI contract before exposing routes.
func NewRouter(users *identity.UserRegistrationService, organizations *organization.OrganizationService, workspaces *workspace.WorkspaceService, agents *agent.AgentDefinitionUseCase, codeRepositories *coderepository.CodeRepositoryService, tasks *task.TaskService, integrations *taskintegration.TaskIntegrationService, mcpServers *mcpserver.MCPServerService, skillCatalog *skill.CatalogService, launches *orchestration.LaunchService, profiles *executionprofile.ExecutionProfileService, runtimeProfiles *agentruntimeprofile.AgentRuntimeProfileService, directChats *orchestration.DirectChatService, workflows *workflow.WorkflowConfigurationService, automation *workflowautomation.WorkflowAutomationService, environment *workspaceenv.WorkspaceEnvironmentService, artifacts *artifact.ArtifactService, agentTools *appmcp.AgentTools) (*chi.Mux, error) {
	if profiles == nil {
		return nil, fmt.Errorf("execution profile service is required")
	}
	if runtimeProfiles == nil {
		return nil, fmt.Errorf("agent runtime profile service is required")
	}
	if launches == nil {
		return nil, fmt.Errorf("task launch service is required")
	}
	if directChats == nil {
		return nil, fmt.Errorf("direct chat service is required")
	}
	if workflows == nil {
		return nil, fmt.Errorf("workflow service is required")
	}
	if integrations == nil {
		return nil, fmt.Errorf("task integration service is required")
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
	if agentTools != nil {
		router.Handle("/mcp/agent-tools", agentTools.Handler())
	}
	router.Group(func(api chi.Router) {
		api.Use(auth.middleware)
		api.Route("/api", func(api chi.Router) {
			api.Get("/me", auth.currentUser)
			api.Get("/runtime-providers", listRuntimeProviders)
			organizationAPI{organizations: organizations, workspaces: workspaces}.routes(api)
			agentAPI{agents: agents}.routes(api)
			executionProfileAPI{service: profiles}.routes(api)
			agentRuntimeProfileAPI{service: runtimeProfiles}.routes(api)
			if skillCatalog != nil {
				skillAPI{service: skillCatalog, scope: skill.ScopeGlobal}.routes(api, "/skills")
				skillAPI{service: skillCatalog, scope: skill.ScopeOrganization}.routes(api, "/organizations/{orgID}/skills")
				skillAPI{service: skillCatalog, scope: skill.ScopeWorkspace}.routes(api, "/organizations/{orgID}/workspaces/{workspaceID}/skills")
			}
			if mcpServers != nil {
				mcpServerAPI{service: mcpServers}.routes(api)
			}
			if environment != nil {
				workspaceEnvironmentAPI{service: environment}.routes(api)
			}
			codeRepositoryAPI{repositories: codeRepositories}.routes(api)
			codingIntegrationAPI{service: codeRepositories}.routes(api)
			taskAPI{tasks: tasks, automation: automation}.routes(api)
			artifactAPI{service: artifacts}.routes(api)
			workflowAPI{service: workflows}.routes(api)
			taskRunAPI{tasks: tasks, launches: launches}.routes(api)
			directChatAPI{chats: directChats, agents: agents, workspaces: workspaces}.routes(api)
			taskIntegrationAPI{service: integrations}.routes(api)
		})
	})
	openapi.Enable(router, "Orkest API", OpenAPIContract)
	mountWeb(router, os.Getenv("ORKEST_WEB_DIR"))
	return router, nil
}
