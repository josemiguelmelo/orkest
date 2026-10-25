package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agent"
	"github.com/josemiguelmelo/orkest/internal/app/httpapi"
	"github.com/josemiguelmelo/orkest/internal/coderepository"
	githubprovider "github.com/josemiguelmelo/orkest/internal/coderepository/providers/github"
	"github.com/josemiguelmelo/orkest/internal/core/httpserver"
	"github.com/josemiguelmelo/orkest/internal/executors/docker"
	"github.com/josemiguelmelo/orkest/internal/identity"
	"github.com/josemiguelmelo/orkest/internal/orchestration"
	"github.com/josemiguelmelo/orkest/internal/organization"
	"github.com/josemiguelmelo/orkest/internal/postgres"
	"github.com/josemiguelmelo/orkest/internal/workspace"
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
	agents := agent.NewService(postgres.NewAgentPostgresRepository(database), workspaces)
	codeRepositories := coderepository.NewService(postgres.NewCodeReposPostgresRepository(database), workspaces, dataRoot, githubprovider.NewGitHubCodeRepositoryProvider())
	var router http.Handler
	if strings.EqualFold(os.Getenv("ORKEST_ENABLE_TEMP_AGENT_LAUNCH"), "true") {
		executor, executorErr := docker.New(ctx, docker.Config{
			DataRoot:    filepath.Join(dataRoot, "agent-executions"),
			ProviderEnv: splitEnvironmentNames(os.Getenv("ORKEST_AGENT_PROVIDER_ENV")),
		})
		if executorErr != nil {
			return fmt.Errorf("initialize temporary Docker agent launcher: %w", executorErr)
		}
		defer func() { _ = executor.Close() }()
		launcher, launcherErr := orchestration.NewAgentLauncher(executor)
		if launcherErr != nil {
			return launcherErr
		}
		launchRepo := postgres.NewAgentLaunchPostgresRepository(database)
		client := orchestration.NewHTTPAgentctlClient(&http.Client{Timeout: 10 * time.Second})
		communication, communicationErr := orchestration.NewConnectionService(ctx, client, launchRepo)
		if communicationErr != nil {
			return communicationErr
		}
		launchService, serviceErr := orchestration.NewLaunchService(launchRepo, launcher, client, communication)
		if serviceErr != nil {
			return serviceErr
		}
		router, err = httpapi.NewRouter(users, organizations, workspaces, agents, codeRepositories, launchService)
	} else {
		router, err = httpapi.NewRouter(users, organizations, workspaces, agents, codeRepositories)
	}
	if err != nil {
		return err
	}

	server := &http.Server{Addr: addr, Handler: router, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	return httpserver.Run(ctx, logger, "platform", server)
}

func splitEnvironmentNames(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.Split(value, ",")
}
