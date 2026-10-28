package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agent"
	"github.com/josemiguelmelo/orkest/internal/app/httpapi"
	"github.com/josemiguelmelo/orkest/internal/coderepository"
	githubprovider "github.com/josemiguelmelo/orkest/internal/coderepository/providers/github"
	gitlabprovider "github.com/josemiguelmelo/orkest/internal/coderepository/providers/gitlab"
	"github.com/josemiguelmelo/orkest/internal/core/httplog"
	"github.com/josemiguelmelo/orkest/internal/core/httpserver"
	"github.com/josemiguelmelo/orkest/internal/core/secretbox"
	"github.com/josemiguelmelo/orkest/internal/executors/docker"
	"github.com/josemiguelmelo/orkest/internal/identity"
	"github.com/josemiguelmelo/orkest/internal/mcpserver"
	"github.com/josemiguelmelo/orkest/internal/orchestration"
	"github.com/josemiguelmelo/orkest/internal/organization"
	"github.com/josemiguelmelo/orkest/internal/postgres"
	"github.com/josemiguelmelo/orkest/internal/skill"
	"github.com/josemiguelmelo/orkest/internal/task"
	"github.com/josemiguelmelo/orkest/internal/taskintegration"
	githubcloud "github.com/josemiguelmelo/orkest/internal/taskintegration/providers/githubcloud"
	jiraserver "github.com/josemiguelmelo/orkest/internal/taskintegration/providers/jiraserver"
	"github.com/josemiguelmelo/orkest/internal/workspace"
	"github.com/josemiguelmelo/orkest/internal/workspaceenv"
)

// Run owns the platform HTTP lifecycle; it does not own agent processes.
func Run(ctx context.Context, logger *slog.Logger) error {
	dataRoot := strings.TrimSpace(os.Getenv("ORKEST_DATA_DIR"))
	if dataRoot == "" {
		dataRoot = "data"
	}
	dataRoot, err := filepath.Abs(dataRoot)
	if err != nil {
		return fmt.Errorf("resolve platform data directory: %w", err)
	}
	if err = os.MkdirAll(dataRoot, 0o700); err != nil {
		return fmt.Errorf("create platform data directory: %w", err)
	}

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}

	database, err := postgres.Initialize(ctx)
	if err != nil {
		return err
	}
	defer database.Close()
	users := identity.NewUserRegistrationService(postgres.NewUserPostgresRepository(database))
	organizations := organization.NewOrganizationService(postgres.NewOrganizationPostgresRepository(database))
	workspaces := workspace.NewWorkspaceService(postgres.NewWorkspacePostgresRepository(database), organizations)
	skillCatalog := skill.NewCatalogService(postgres.NewSkillCatalogPostgresRepository(database), organizations, workspaces, users)
	var secretBox *secretbox.Box
	if key := os.Getenv("ORKEST_INTEGRATION_ENCRYPTION_KEY"); key != "" {
		secretBox, err = secretbox.New(key)
		if err != nil {
			return err
		}
	}
	mcpServerService := mcpserver.NewMCPServerService(postgres.NewMCPServerPostgresRepository(database), secretBox, workspaces)
	workspaceEnvironment := workspaceenv.NewService(postgres.NewWorkspaceEnvironmentPostgresRepository(database), workspaces, secretBox)
	agents := agent.NewService(postgres.NewAgentPostgresRepository(database), workspaces, mcpServerService, skillCatalog)
	codeRepositories := coderepository.NewService(postgres.NewCodeReposPostgresRepository(database), workspaces, dataRoot, githubprovider.NewGitHubProvider(), gitlabprovider.NewGitLabCloudProvider(), gitlabprovider.NewGitLabServerProvider())
	codeRepositories.SetSecretBox(secretBox)
	launchRepo := postgres.NewAgentLaunchPostgresRepository(database, secretBox)
	client := orchestration.NewHTTPAgentctlClient(httplog.PropagatingHTTPClient(&http.Client{Timeout: 10 * time.Second}))
	executor, executorErr := docker.New(ctx, docker.Config{
		DataRoot:                 filepath.Join(dataRoot, "agent-executions"),
		ProviderEnv:              splitEnvironmentNames(os.Getenv("ORKEST_AGENT_PROVIDER_ENV")),
		CodingProviders:          []coderepository.CodingProvider{githubprovider.NewGitHubProvider(), gitlabprovider.NewGitLabCloudProvider(), gitlabprovider.NewGitLabServerProvider()},
		CodingCredentialResolver: codeRepositories,
	})
	if executorErr != nil {
		return fmt.Errorf("initialize Docker agent executor: %w", executorErr)
	}
	defer func() { _ = executor.Close() }()
	completion, completionErr := orchestration.NewLaunchCompletionService(launchRepo, executor)
	if completionErr != nil {
		return completionErr
	}
	if err = completion.RecoverPending(ctx); err != nil {
		return err
	}
	communication, communicationErr := orchestration.NewConnectionService(ctx, client, launchRepo, completion)
	if communicationErr != nil {
		return communicationErr
	}
	if err = communication.ReconnectRecorded(); err != nil {
		return err
	}
	launcher, err := orchestration.NewAgentLauncher(executor)
	if err != nil {
		return err
	}
	launchService, err := orchestration.NewLaunchService(launchRepo, launcher, client, communication, completion)
	if err != nil {
		return err
	}
	dispatcher, err := orchestration.NewTaskDispatcher(agents, launchService, mcpServerService)
	if err != nil {
		return err
	}
	dispatcher.SetRepositoryResolver(codeRepositories)
	dispatcher.SetCodingCredentialResolver(codeRepositories)
	dispatcher.SetSkillResolver(skillCatalog)
	dispatcher.SetEnvironmentResolver(workspaceEnvironment)
	tasks := task.NewService(postgres.NewTaskPostgresRepository(database), workspaces, dispatcher)
	integrations := taskintegration.NewTaskIntegrationService(postgres.NewPostgresTaskIntegrationRepository(database), secretBox, workspaces, organizations, postgres.NewPostgresImportedTaskRepository(database), githubcloud.NewGitHubCloudProvider(), jiraserver.NewJiraServerProvider())
	runnerCtx, stopRunner := context.WithCancel(ctx)
	var runnerWG sync.WaitGroup
	runnerWG.Add(1)
	go func() { defer runnerWG.Done(); integrations.Run(runnerCtx) }()
	router, err := httpapi.NewRouter(users, organizations, workspaces, agents, codeRepositories, tasks, integrations, mcpServerService, skillCatalog, launchService, workspaceEnvironment)
	if err != nil {
		stopRunner()
		runnerWG.Wait()
		return err
	}

	server := &http.Server{Addr: addr, Handler: httplog.Middleware(logger)(router), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	serverErr := httpserver.Run(ctx, logger, "platform", server)
	stopRunner()
	runnerWG.Wait()
	return serverErr
}

func splitEnvironmentNames(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.Split(value, ",")
}
