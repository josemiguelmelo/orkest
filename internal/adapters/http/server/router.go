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

// RouterDependencies names the services required by platform routes.
type RouterDependencies struct {
	Users            *identity.UserRegistrationService
	Organizations    *organization.OrganizationService
	Workspaces       *workspace.WorkspaceService
	Agents           *agent.AgentDefinitionUseCase
	CodeRepositories *coderepository.CodeRepositoryService
	Tasks            *task.TaskService
	Integrations     *taskintegration.TaskIntegrationService
	MCPServers       *mcpserver.MCPServerService
	SkillCatalog     *skill.CatalogService
	Launches         *orchestration.LaunchService
	Profiles         *executionprofile.ExecutionProfileService
	RuntimeProfiles  *agentruntimeprofile.AgentRuntimeProfileService
	DirectChats      *orchestration.DirectChatService
	Workflows        *workflow.WorkflowConfigurationService
	Automation       *workflowautomation.WorkflowAutomationService
	Environment      *workspaceenv.WorkspaceEnvironmentService
	Artifacts        *artifact.ArtifactService
	AgentTools       *appmcp.AgentTools
}

// NewRouter validates the served OpenAPI contract before exposing routes.
func NewRouter(ctx context.Context, deps RouterDependencies) (*chi.Mux, error) {
	if deps.Profiles == nil {
		return nil, fmt.Errorf("execution profile service is required")
	}
	if deps.RuntimeProfiles == nil {
		return nil, fmt.Errorf("agent runtime profile service is required")
	}
	if deps.Launches == nil {
		return nil, fmt.Errorf("task launch service is required")
	}
	if deps.DirectChats == nil {
		return nil, fmt.Errorf("direct chat service is required")
	}
	if deps.Workflows == nil {
		return nil, fmt.Errorf("workflow service is required")
	}
	if deps.Integrations == nil {
		return nil, fmt.Errorf("task integration service is required")
	}
	if _, err := openapi.LoadAndValidateOpenAPIDoc(OpenAPIContract, ctx); err != nil {
		return nil, fmt.Errorf("load platform OpenAPI contract: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	auth, err := newOIDCAuth(ctx, deps.Users, deps.Organizations)
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
	if deps.AgentTools != nil {
		router.Handle("/mcp/agent-tools", deps.AgentTools.Handler())
	}
	router.Group(func(api chi.Router) {
		api.Use(auth.middleware)
		api.Route("/api", func(api chi.Router) {
			api.Get("/me", auth.currentUser)
			api.Get("/runtime-providers", listRuntimeProviders)
			organizationAPI{organizations: deps.Organizations, workspaces: deps.Workspaces}.routes(api)
			agentAPI{agents: deps.Agents}.routes(api)
			executionProfileAPI{service: deps.Profiles}.routes(api)
			agentRuntimeProfileAPI{service: deps.RuntimeProfiles}.routes(api)
			if deps.SkillCatalog != nil {
				skillAPI{service: deps.SkillCatalog, scope: skill.ScopeGlobal}.routes(api, "/skills")
				skillAPI{service: deps.SkillCatalog, scope: skill.ScopeOrganization}.routes(api, "/organizations/{orgID}/skills")
				skillAPI{service: deps.SkillCatalog, scope: skill.ScopeWorkspace}.routes(api, "/organizations/{orgID}/workspaces/{workspaceID}/skills")
			}
			if deps.MCPServers != nil {
				mcpServerAPI{service: deps.MCPServers}.routes(api)
			}
			if deps.Environment != nil {
				workspaceEnvironmentAPI{service: deps.Environment}.routes(api)
			}
			codeRepositoryAPI{repositories: deps.CodeRepositories}.routes(api)
			codingIntegrationAPI{service: deps.CodeRepositories}.routes(api)
			taskAPI{tasks: deps.Tasks, automation: deps.Automation}.routes(api)
			artifactAPI{service: deps.Artifacts}.routes(api)
			workflowAPI{service: deps.Workflows}.routes(api)
			taskRunAPI{tasks: deps.Tasks, launches: deps.Launches}.routes(api)
			directChatAPI{chats: deps.DirectChats, agents: deps.Agents, workspaces: deps.Workspaces}.routes(api)
			taskIntegrationAPI{service: deps.Integrations}.routes(api)
		})
	})
	openapi.Enable(router, "Orkest API", OpenAPIContract)
	mountWeb(router, os.Getenv("ORKEST_WEB_DIR"))
	return router, nil
}
