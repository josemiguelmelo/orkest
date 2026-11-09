package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/josemiguelmelo/orkest/internal/adapters/runnerclient/agentctlhttp"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors"
	kubernetesexecutor "github.com/josemiguelmelo/orkest/internal/adapters/executors/kubernetes"
	"github.com/josemiguelmelo/orkest/internal/adapters/http/server"
	"github.com/josemiguelmelo/orkest/internal/adapters/imagebuild"
	"github.com/josemiguelmelo/orkest/internal/adapters/imagebuild/buildkit"
	dockerimage "github.com/josemiguelmelo/orkest/internal/adapters/imagebuild/docker"
	githubprovider "github.com/josemiguelmelo/orkest/internal/adapters/integrations/coderepository/providers/github"
	gitlabprovider "github.com/josemiguelmelo/orkest/internal/adapters/integrations/coderepository/providers/gitlab"
	githubcloud "github.com/josemiguelmelo/orkest/internal/adapters/integrations/taskintegration/providers/githubcloud"
	jiraserver "github.com/josemiguelmelo/orkest/internal/adapters/integrations/taskintegration/providers/jiraserver"
	githubtrigger "github.com/josemiguelmelo/orkest/internal/adapters/integrations/tasktriggerintegration/providers/github"
	appmcp "github.com/josemiguelmelo/orkest/internal/adapters/mcp"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/platform/httplog"
	"github.com/josemiguelmelo/orkest/internal/platform/httpserver"
	"github.com/josemiguelmelo/orkest/internal/platform/secretbox"
	"github.com/josemiguelmelo/orkest/internal/server/agent"
	"github.com/josemiguelmelo/orkest/internal/server/agentruntimeprofile"
	"github.com/josemiguelmelo/orkest/internal/server/artifact"
	"github.com/josemiguelmelo/orkest/internal/server/coderepository"
	"github.com/josemiguelmelo/orkest/internal/server/executionprofile"
	"github.com/josemiguelmelo/orkest/internal/server/identity"
	"github.com/josemiguelmelo/orkest/internal/server/mcpserver"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
	"github.com/josemiguelmelo/orkest/internal/server/organization"
	"github.com/josemiguelmelo/orkest/internal/server/pullrequestlink"
	"github.com/josemiguelmelo/orkest/internal/server/skill"
	"github.com/josemiguelmelo/orkest/internal/server/task"
	"github.com/josemiguelmelo/orkest/internal/server/taskintegration"
	"github.com/josemiguelmelo/orkest/internal/server/tasktriggerintegration"
	"github.com/josemiguelmelo/orkest/internal/server/workflow"
	workflowautomation "github.com/josemiguelmelo/orkest/internal/server/workflowautomation"
	"github.com/josemiguelmelo/orkest/internal/server/workspace"
	"github.com/josemiguelmelo/orkest/internal/server/workspaceenv"
)

func parsePositiveInt64(value string, fallback int64) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseDuration(value string, fallback time.Duration) time.Duration {
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func kubernetesAgentMCPURL(config kubernetesServerConfiguration) (string, error) {
	namespace := strings.TrimSpace(config.Namespace)
	if namespace == "" {
		return "", fmt.Errorf("ORKEST_KUBERNETES_NAMESPACE is required to derive the worker MCP endpoint")
	}
	service := strings.TrimSpace(config.PlatformServiceName)
	if service == "" {
		service = "orkest"
	}
	if !isDNSLabel(service) || !isDNSLabel(namespace) {
		return "", fmt.Errorf("kubernetes platform service name and namespace must be DNS labels")
	}
	port := int64(8080)
	if value := strings.TrimSpace(config.PlatformServicePort); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed <= 0 || parsed > 65535 {
			return "", fmt.Errorf("ORKEST_KUBERNETES_PLATFORM_SERVICE_PORT must be between 1 and 65535")
		}
		port = parsed
	}
	return fmt.Sprintf("http://%s.%s.svc:%d", service, namespace, port), nil
}

func isDNSLabel(value string) bool {
	if value == "" || len(value) > 63 {
		return false
	}
	valid := func(r rune) bool { return r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' }
	for _, r := range value {
		if !valid(r) {
			return false
		}
	}
	first := rune(value[0])
	return (first >= 'a' && first <= 'z' || first >= '0' && first <= '9') && value[len(value)-1] != '-'
}

// Run owns the platform HTTP lifecycle; it does not own agent processes.
func runServer(ctx context.Context, logger *slog.Logger) error {
	config := loadServerConfiguration()
	kubernetesExecutor := strings.EqualFold(strings.TrimSpace(config.Executor), "kubernetes")
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	dataRoot := strings.TrimSpace(config.DataDir)
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

	addr := config.HTTPAddr
	if addr == "" {
		addr = "127.0.0.1:8080"
		if kubernetesExecutor {
			addr = "0.0.0.0:8080"
		}
	}
	if kubernetesExecutor {
		host, _, splitErr := net.SplitHostPort(addr)
		if splitErr != nil {
			return fmt.Errorf("HTTP_ADDR must use host:port format for Kubernetes service access: %w", splitErr)
		}
		if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil && ip.IsLoopback() || strings.EqualFold(host, "localhost") {
			return fmt.Errorf("HTTP_ADDR must bind to a non-loopback interface when ORKEST_EXECUTOR=kubernetes")
		}
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
	if key := config.IntegrationEncryptionKey; key != "" {
		secretBox, err = secretbox.New(key)
		if err != nil {
			return err
		}
	}
	mcpServerService := mcpserver.NewMCPServerService(postgres.NewMCPServerPostgresRepository(database), secretBox, workspaces)
	workspaceEnvironment := workspaceenv.NewWorkspaceEnvironmentService(postgres.NewWorkspaceEnvironmentPostgresRepository(database), workspaces, secretBox)
	executionProfileRepository := postgres.NewExecutionProfileRepository(database)
	profileImages := map[coreruntime.Runtime]string{coreruntime.OpenCode: config.AgentctlOpencodeImage, coreruntime.ClaudeCode: config.AgentctlClaudeCodeImage, coreruntime.Codex: config.AgentctlCodexImage, coreruntime.GitHubCopilot: config.AgentctlGitHubCopilotImage, coreruntime.Kiro: config.AgentctlKiroImage}
	executionProfileService := executionprofile.NewExecutionProfileService(executionProfileRepository, workspaces, config.ExecutionImageRepository, profileImages)
	agentRepository := postgres.NewAgentPostgresRepository(database, secretBox)
	agentRuntimeProfileRepository := postgres.NewAgentRuntimeProfilePostgresRepository(database, secretBox)
	agentRuntimeProfiles := agentruntimeprofile.NewAgentRuntimeProfileService(agentRuntimeProfileRepository, workspaces)
	agents := agent.NewAgentDefinitionUseCase(agentRepository, workspaces, mcpServerService, skillCatalog, executionProfileService, agentRuntimeProfiles)
	codeRepositories := coderepository.NewCodeRepositoryService(postgres.NewCodeReposPostgresRepository(database), workspaces, dataRoot, secretBox, githubprovider.NewGitHubProvider(), gitlabprovider.NewGitLabCloudProvider(), gitlabprovider.NewGitLabServerProvider())
	launchRepo := postgres.NewAgentLaunchPostgresRepository(database, secretBox)
	client := agentctlhttp.NewHTTPAgentctlClient(httplog.PropagatingHTTPClient(&http.Client{Timeout: 10 * time.Second}))
	providers := []coderepository.CodingProvider{githubprovider.NewGitHubProvider(), gitlabprovider.NewGitLabCloudProvider(), gitlabprovider.NewGitLabServerProvider()}
	executorSelection, executorErr := executors.New(ctx, executors.Config{
		Kind:                     config.Executor,
		DataRoot:                 filepath.Join(dataRoot, "agent-executions"),
		DockerNetwork:            config.AgentctlNetwork,
		CodexAuthFile:            config.CodexAuthFile,
		AgentctlBinary:           config.AgentctlBinary,
		DockerImages:             profileImages,
		ProviderEnv:              splitEnvironmentNames(config.AgentProviderEnv),
		CodingProviders:          providers,
		CodingCredentialResolver: codeRepositories,
		Kubernetes: kubernetesexecutor.Config{
			Namespace:               config.Kubernetes.Namespace,
			InstallationID:          config.Kubernetes.InstallationID,
			DataPVC:                 config.Kubernetes.DataPVC,
			DataSubpath:             config.Kubernetes.DataSubpath,
			ServiceAccount:          config.Kubernetes.ServiceAccount,
			UID:                     parsePositiveInt64(config.Kubernetes.UID, int64(os.Getuid())),
			GID:                     parsePositiveInt64(config.Kubernetes.GID, int64(os.Getgid())),
			StartupTimeout:          parseDuration(config.Kubernetes.StartupTimeout, 2*time.Minute),
			TerminationGraceSeconds: parsePositiveInt64(config.Kubernetes.TerminationGrace, 10),
			CodexAuthSecret:         config.Kubernetes.CodexAuthSecret,
			CodexAuthSecretKey:      config.Kubernetes.CodexAuthSecretKey,
			ImagePullSecrets:        splitEnvironmentNames(config.Kubernetes.ImagePullSecrets),
			CPURequest:              config.Kubernetes.CPURequest,
			MemoryRequest:           config.Kubernetes.MemoryRequest,
			CPULimit:                config.Kubernetes.CPULimit,
			MemoryLimit:             config.Kubernetes.MemoryLimit,
		},
	})
	if executorErr != nil {
		return fmt.Errorf("initialize agent executor: %w", executorErr)
	}
	executor := executorSelection.Executor
	executorKind := executorSelection.Kind
	defer func() { _ = executor.Close() }()
	completion, completionErr := orchestration.NewLaunchCompletionService(ctx, launchRepo, executor)
	if completionErr != nil {
		return completionErr
	}
	defer completion.Close()
	if err = completion.RecoverPending(ctx); err != nil {
		return err
	}
	communication, communicationErr := orchestration.NewConnectionService(ctx, client, launchRepo, completion)
	if communicationErr != nil {
		return communicationErr
	}
	defer communication.Close()
	defer cancel()
	if err = communication.ReconnectRecorded(); err != nil {
		return err
	}
	launchService, err := orchestration.NewLaunchService(launchRepo, executor, client, communication, completion)
	if err != nil {
		return err
	}
	taskRepository := postgres.NewTaskPostgresRepository(database)
	workflows := workflow.NewWorkflowConfigurationService(postgres.NewWorkflowDefinitionPostgresRepository(database), workspaces)
	taskWorkflows := workflowautomation.NewWorkflowAutomationService(postgres.NewTaskTransitionPostgresRepository(database), postgres.NewAutomationEventPostgresRepository(database), workspaces, workflowautomation.WorkflowRuleEvaluator{})
	tasks := task.NewTaskService(taskRepository, postgres.NewTaskCreationPostgresRepository(database), postgres.NewTaskUpdatePostgresRepository(database), taskWorkflows, workspaces)
	artifacts := artifact.NewArtifactService(postgres.NewArtifactPostgresRepository(database), tasks)
	mcpEndpoint := strings.TrimSpace(config.AgentMCPURL)
	if kubernetesExecutor {
		mcpEndpoint, err = kubernetesAgentMCPURL(config.Kubernetes)
		if err != nil {
			return err
		}
	} else if mcpEndpoint == "" {
		mcpEndpoint = "http://127.0.0.1:8080"
	}
	mcpKey, err := appmcp.AgentToolsTokenKey(dataRoot, config.AgentMCPTokenKey)
	if err != nil {
		return fmt.Errorf("load agent MCP token key: %w", err)
	}
	agentTools, err := appmcp.NewAgentTools(mcpKey, mcpEndpoint, codeRepositories, tasks, artifacts)
	if err != nil {
		return fmt.Errorf("initialize agent MCP tools: %w", err)
	}
	var profileResolver orchestration.ExecutionImageResolver = executionProfileService
	if executorKind == executors.Local {
		profileResolver = localExecutionProfileResolver{}
	}
	dispatcher, err := orchestration.NewTaskDispatcher(orchestration.TaskDispatcherDependencies{
		Agents:       agents,
		Launches:     launchService,
		Sessions:     launchRepo,
		MCPServers:   mcpServerService,
		Profiles:     profileResolver,
		Skills:       skillCatalog,
		Environment:  workspaceEnvironment,
		Repositories: codeRepositories,
		Credentials:  codeRepositories,
		Tools:        agentTools,
		Artifacts:    artifacts,
		Workflows:    workflows,
	})
	if err != nil {
		return err
	}
	directChats, err := orchestration.NewDirectChatService(launchRepo, dispatcher, launchService)
	if err != nil {
		return err
	}
	integrations := taskintegration.NewTaskIntegrationService(postgres.NewPostgresTaskIntegrationRepository(database), secretBox, workspaces, organizations, postgres.NewPostgresImportedTaskRepository(database), tasks, githubcloud.NewGitHubCloudProvider(), jiraserver.NewJiraServerProvider())
	prLinkRepository := postgres.NewPullRequestLinkRepository(database)
	prLinks := pullrequestlink.NewPullRequestLinkService(prLinkRepository)
	triggerIntegrations, err := tasktriggerintegration.NewTaskTriggerIntegrationService(postgres.NewTaskTriggerIntegrationPostgresRepository(database), workspaces, secretBox, prLinks, githubtrigger.NewGitHubProvider())
	if err != nil {
		return err
	}
	runnerCtx, stopRunner := context.WithCancel(ctx)
	var runnerWG sync.WaitGroup
	if config.ExecutionImageRepository != "" && executorKind != executors.Kubernetes {
		registry := imagebuild.RegistryMapping{
			DockerHost:   config.DockerRegistryHost,
			BuildKitHost: config.BuildKitRegistryHost,
		}
		imageStore, buildErr := dockerimage.NewImageStore(registry)
		if buildErr != nil {
			stopRunner()
			return buildErr
		}
		builder, buildErr := newImageBuilder(config.ImageBuilder, imageStore, buildkit.Config{
			Binary:     config.BuildctlBinary,
			Address:    config.BuildKitAddr,
			Registry:   registry,
			TLSCA:      config.BuildKitTLSCA,
			TLSCert:    config.BuildKitTLSCert,
			TLSKey:     config.BuildKitTLSKey,
			Daemonless: strings.EqualFold(config.BuildKitDaemonless, "true"),
		})
		if buildErr != nil {
			stopRunner()
			return buildErr
		}
		workerCount := 1
		if configured, parseErr := strconv.Atoi(config.ExecutionProfileBuildWorkers); parseErr == nil && configured > 0 {
			workerCount = configured
		} else if config.ExecutionProfileBuildWorkers != "" {
			logger.Warn("invalid execution profile build worker count; using default", "setting", "ORKEST_EXECUTION_PROFILE_BUILD_WORKERS", "default", workerCount)
		}
		worker := executionprofile.NewExecutionProfileWorker(executionProfileRepository, builder, logger, workerCount)
		runnerWG.Add(1)
		go func() { defer runnerWG.Done(); worker.Run(runnerCtx) }()
	} else if config.ExecutionImageRepository != "" {
		logger.Warn("execution profile builds are disabled for the Kubernetes executor until the Kubernetes image builder is configured", "executor", executorKind)
	}
	runnerWG.Add(1)
	go func() { defer runnerWG.Done(); integrations.Run(runnerCtx) }()
	runnerWG.Add(1)
	go func() { defer runnerWG.Done(); triggerIntegrations.Run(runnerCtx, logger) }()
	prReconciliation := pullrequestlink.NewReconciler(prLinkRepository, triggerIntegrations, map[string]pullrequestlink.Discoverer{"github": githubtrigger.Discovery{Credentials: codeRepositories}})
	runnerWG.Add(1)
	go func() { defer runnerWG.Done(); prReconciliation.Run(runnerCtx, logger) }()
	runnerWG.Add(1)
	go func() { defer runnerWG.Done(); prLinks.Run(runnerCtx, logger) }()
	execution := workflowautomation.NewWorkflowExecutionService(postgres.NewExecutionCommandPostgresRepository(database), tasks, dispatcher, logger)
	worker := workflowautomation.NewWorkflowAutomationWorker(taskWorkflows, execution, logger)
	runnerWG.Add(1)
	go func() { defer runnerWG.Done(); worker.Run(runnerCtx) }()
	router, err := httpapi.NewRouter(ctx, httpapi.RouterDependencies{
		Users:               users,
		Organizations:       organizations,
		Workspaces:          workspaces,
		Agents:              agents,
		CodeRepositories:    codeRepositories,
		Tasks:               tasks,
		Integrations:        integrations,
		TriggerIntegrations: triggerIntegrations,
		MCPServers:          mcpServerService,
		SkillCatalog:        skillCatalog,
		Launches:            launchService,
		Profiles:            executionProfileService,
		RuntimeProfiles:     agentRuntimeProfiles,
		DirectChats:         directChats,
		Workflows:           workflows,
		Automation:          taskWorkflows,
		Environment:         workspaceEnvironment,
		Artifacts:           artifacts,
		AgentTools:          agentTools,
	})
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
