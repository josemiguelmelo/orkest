package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agent"
	"github.com/josemiguelmelo/orkest/internal/agentruntimeprofile"
	"github.com/josemiguelmelo/orkest/internal/app/httpapi"
	"github.com/josemiguelmelo/orkest/internal/coderepository"
	githubprovider "github.com/josemiguelmelo/orkest/internal/coderepository/providers/github"
	gitlabprovider "github.com/josemiguelmelo/orkest/internal/coderepository/providers/gitlab"
	"github.com/josemiguelmelo/orkest/internal/core/httplog"
	"github.com/josemiguelmelo/orkest/internal/core/httpserver"
	coreruntime "github.com/josemiguelmelo/orkest/internal/core/runtime"
	"github.com/josemiguelmelo/orkest/internal/core/secretbox"
	"github.com/josemiguelmelo/orkest/internal/executionprofile"
	"github.com/josemiguelmelo/orkest/internal/executors"
	"github.com/josemiguelmelo/orkest/internal/identity"
	"github.com/josemiguelmelo/orkest/internal/imagebuild"
	"github.com/josemiguelmelo/orkest/internal/imagebuild/buildkit"
	dockerimage "github.com/josemiguelmelo/orkest/internal/imagebuild/docker"
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
	executionProfileRepository := postgres.NewExecutionProfileRepository(database)
	profileImages := map[coreruntime.Runtime]string{coreruntime.OpenCode: os.Getenv("AGENTCTL_OPENCODE_IMAGE"), coreruntime.ClaudeCode: os.Getenv("AGENTCTL_CLAUDE_CODE_IMAGE"), coreruntime.Codex: os.Getenv("AGENTCTL_CODEX_IMAGE"), coreruntime.GitHubCopilot: os.Getenv("AGENTCTL_GITHUB_COPILOT_IMAGE"), coreruntime.Kiro: os.Getenv("AGENTCTL_KIRO_IMAGE")}
	executionProfileService := executionprofile.NewExecutionProfileService(executionProfileRepository, workspaces, os.Getenv("ORKEST_EXECUTION_IMAGE_REPOSITORY"), profileImages)
	agentRepository := postgres.NewAgentPostgresRepository(database)
	agentRepository.SetSecretBox(secretBox)
	agents := agent.NewService(agentRepository, workspaces, mcpServerService, skillCatalog)
	agents.SetProfiles(executionProfileService)
	agentRuntimeProfileRepository := postgres.NewAgentRuntimeProfilePostgresRepository(database)
	agentRuntimeProfileRepository.SetSecretBox(secretBox)
	agentRuntimeProfiles := agentruntimeprofile.NewService(agentRuntimeProfileRepository, workspaces)
	agents.SetRuntimeProfiles(agentRuntimeProfiles)
	codeRepositories := coderepository.NewService(postgres.NewCodeReposPostgresRepository(database), workspaces, dataRoot, githubprovider.NewGitHubProvider(), gitlabprovider.NewGitLabCloudProvider(), gitlabprovider.NewGitLabServerProvider())
	codeRepositories.SetSecretBox(secretBox)
	launchRepo := postgres.NewAgentLaunchPostgresRepository(database, secretBox)
	client := orchestration.NewHTTPAgentctlClient(httplog.PropagatingHTTPClient(&http.Client{Timeout: 10 * time.Second}))
	providers := []coderepository.CodingProvider{githubprovider.NewGitHubProvider(), gitlabprovider.NewGitLabCloudProvider(), gitlabprovider.NewGitLabServerProvider()}
	executorSelection, executorErr := executors.New(ctx, executors.Config{
		Kind:           os.Getenv("ORKEST_EXECUTOR"),
		DataRoot:       filepath.Join(dataRoot, "agent-executions"),
		DockerNetwork:  os.Getenv("ORKEST_AGENTCTL_NETWORK"),
		CodexAuthFile:  os.Getenv("ORKEST_CODEX_AUTH_FILE"),
		AgentctlBinary: os.Getenv("ORKEST_AGENTCTL_BINARY"),
		DockerImages: map[coreruntime.Runtime]string{
			coreruntime.OpenCode:      os.Getenv("AGENTCTL_OPENCODE_IMAGE"),
			coreruntime.ClaudeCode:    os.Getenv("AGENTCTL_CLAUDE_CODE_IMAGE"),
			coreruntime.Codex:         os.Getenv("AGENTCTL_CODEX_IMAGE"),
			coreruntime.GitHubCopilot: os.Getenv("AGENTCTL_GITHUB_COPILOT_IMAGE"),
			coreruntime.Kiro:          os.Getenv("AGENTCTL_KIRO_IMAGE"),
		},
		ProviderEnv:              splitEnvironmentNames(os.Getenv("ORKEST_AGENT_PROVIDER_ENV")),
		CodingProviders:          providers,
		CodingCredentialResolver: codeRepositories,
	})
	if executorErr != nil {
		return fmt.Errorf("initialize agent executor: %w", executorErr)
	}
	executor := executorSelection.Executor
	executorKind := executorSelection.Kind
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
	if executorKind == executors.Local {
		dispatcher.SetProfiles(localExecutionProfileResolver{})
	} else {
		dispatcher.SetProfiles(executionProfileService)
	}
	dispatcher.SetRepositoryResolver(codeRepositories)
	dispatcher.SetCodingCredentialResolver(codeRepositories)
	dispatcher.SetSkillResolver(skillCatalog)
	dispatcher.SetEnvironmentResolver(workspaceEnvironment)
	directChats, err := orchestration.NewDirectChatService(launchRepo, dispatcher, launchService)
	if err != nil {
		return err
	}
	tasks := task.NewService(postgres.NewTaskPostgresRepository(database), workspaces, dispatcher)
	integrations := taskintegration.NewTaskIntegrationService(postgres.NewPostgresTaskIntegrationRepository(database), secretBox, workspaces, organizations, postgres.NewPostgresImportedTaskRepository(database), githubcloud.NewGitHubCloudProvider(), jiraserver.NewJiraServerProvider())
	runnerCtx, stopRunner := context.WithCancel(ctx)
	var runnerWG sync.WaitGroup
	if os.Getenv("ORKEST_EXECUTION_IMAGE_REPOSITORY") != "" {
		registry := imagebuild.RegistryMapping{
			DockerHost:   os.Getenv("ORKEST_DOCKER_REGISTRY_HOST"),
			BuildKitHost: os.Getenv("ORKEST_BUILDKIT_REGISTRY_HOST"),
		}
		imageStore, buildErr := dockerimage.NewImageStore(registry)
		if buildErr != nil {
			stopRunner()
			return buildErr
		}
		builder, buildErr := newImageBuilder(os.Getenv("ORKEST_IMAGE_BUILDER"), imageStore, buildkit.Config{
			Binary:     os.Getenv("ORKEST_BUILDCTL_BINARY"),
			Address:    os.Getenv("ORKEST_BUILDKIT_ADDR"),
			Registry:   registry,
			TLSCA:      os.Getenv("ORKEST_BUILDKIT_TLS_CA"),
			TLSCert:    os.Getenv("ORKEST_BUILDKIT_TLS_CERT"),
			TLSKey:     os.Getenv("ORKEST_BUILDKIT_TLS_KEY"),
			Daemonless: strings.EqualFold(os.Getenv("ORKEST_BUILDKIT_DAEMONLESS"), "true"),
		})
		if buildErr != nil {
			stopRunner()
			return buildErr
		}
		workerCount := 1
		if configured, parseErr := strconv.Atoi(os.Getenv("ORKEST_EXECUTION_PROFILE_BUILD_WORKERS")); parseErr == nil && configured > 0 {
			workerCount = configured
		} else if os.Getenv("ORKEST_EXECUTION_PROFILE_BUILD_WORKERS") != "" {
			logger.Warn("invalid execution profile build worker count; using default", "setting", "ORKEST_EXECUTION_PROFILE_BUILD_WORKERS", "default", workerCount)
		}
		worker := executionprofile.NewExecutionProfileWorker(executionProfileRepository, builder, logger, workerCount)
		runnerWG.Add(1)
		go func() { defer runnerWG.Done(); worker.Run(runnerCtx) }()
	}
	runnerWG.Add(1)
	go func() { defer runnerWG.Done(); integrations.Run(runnerCtx) }()
	router, err := httpapi.NewRouter(users, organizations, workspaces, agents, codeRepositories, tasks, integrations, mcpServerService, skillCatalog, launchService, executionProfileService, agentRuntimeProfiles, directChats, workspaceEnvironment)
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

type localExecutionProfileResolver struct{}

func (localExecutionProfileResolver) Resolve(context.Context, string, string, coreruntime.Runtime) (string, error) {
	return "", nil
}

func splitEnvironmentNames(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.Split(value, ",")
}
