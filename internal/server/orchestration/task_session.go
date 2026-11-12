package orchestration

import (
	"context"
	"errors"
	"fmt"
	"strings"

	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	agentmodel "github.com/josemiguelmelo/orkest/internal/server/agent/model"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
)

func (d *TaskDispatcher) createTaskSession(ctx context.Context, userID, organizationID string, value taskmodel.Task, runID string) (orchestrationmodel.RunConfig, error) {
	config, definition, err := d.freshConfiguration(ctx, userID, organizationID, value)
	if err != nil {
		return orchestrationmodel.RunConfig{}, err
	}
	config.TaskSessionID, err = ids.NewUUID()
	if err != nil {
		return orchestrationmodel.RunConfig{}, err
	}
	config.ExecutionProfileID = definition.ExecutionProfileID
	if err = d.resolveHostMounts(ctx, &config); err != nil {
		return orchestrationmodel.RunConfig{}, err
	}
	if err = d.resolveExecutionImage(ctx, value.WorkspaceID, &config); err != nil {
		return orchestrationmodel.RunConfig{}, err
	}
	config.RunID, config.RestoreSession = runID, false
	config.GitCredentials = nil
	config.GitCredentialsRequired = false
	config.Repositories = nil

	if d.environmentResolver != nil {
		config.Environment, err = d.environmentResolver.Resolve(ctx, value.WorkspaceID)
		if err != nil {
			return orchestrationmodel.RunConfig{}, fmt.Errorf("resolve workspace environment: %w", err)
		}
	}
	if config.Environment == nil {
		config.Environment = make(map[string]string)
	}
	for name, value := range definition.Configuration {
		config.Environment[name] = value
	}
	if err = coreruntime.ValidateRequiredEnvironment(config.Runtime, config.Environment); err != nil {
		return orchestrationmodel.RunConfig{}, fmt.Errorf("resolve runtime configuration: %w", err)
	}
	if err = d.resolveTaskRepositories(ctx, userID, organizationID, value, &config); err != nil {
		return orchestrationmodel.RunConfig{}, err
	}
	if err := d.resolveAgentCapabilities(ctx, organizationID, value.WorkspaceID, definition, &config); err != nil {
		return orchestrationmodel.RunConfig{}, err
	}
	return config, nil
}

func (d *TaskDispatcher) continueTaskSession(ctx context.Context, userID, organizationID string, value taskmodel.Task, runID string) (orchestrationmodel.RunConfig, bool, error) {
	config, found, err := d.sessions.CurrentSession(ctx, organizationID, value.WorkspaceID, value.ID)
	if err != nil || !found {
		return config, found, err
	}
	if err = d.resolveExecutionImage(ctx, value.WorkspaceID, &config); err != nil {
		return orchestrationmodel.RunConfig{}, false, err
	}
	config.RunID, config.RestoreSession = runID, true
	config.GitCredentials = nil
	config.Repositories = nil
	if d.environmentResolver != nil {
		environment, resolveErr := d.environmentResolver.Resolve(ctx, value.WorkspaceID)
		if resolveErr != nil {
			return orchestrationmodel.RunConfig{}, false, fmt.Errorf("resolve workspace environment: %w", resolveErr)
		}
		if environment == nil {
			environment = make(map[string]string, len(config.Environment))
		}
		for name, oldValue := range config.Environment {
			if _, exists := environment[name]; !exists {
				environment[name] = oldValue
			}
		}
		config.Environment = environment
	}
	if err = d.resolveTaskRepositories(ctx, userID, organizationID, value, &config); err != nil {
		return orchestrationmodel.RunConfig{}, false, err
	}
	if err := d.refreshSessionMCPServers(ctx, value.WorkspaceID, &config); err != nil {
		return orchestrationmodel.RunConfig{}, false, err
	}

	return config, true, nil
}

func (d *TaskDispatcher) resolveTaskRepositories(ctx context.Context, userID, organizationID string, value taskmodel.Task, config *orchestrationmodel.RunConfig) error {
	config.Repositories = nil
	config.GitCredentials = nil
	if d.repositories == nil {
		return d.resolveWorkspaceGitCredentials(ctx, userID, organizationID, value.WorkspaceID, config)
	}
	repositories, err := d.repositories.List(ctx, userID, organizationID, value.WorkspaceID)
	if err != nil {
		return fmt.Errorf("resolve workspace repositories: %w", err)
	}
	selectedRepositories := make(map[string]struct{}, len(value.Repositories))
	for _, repository := range value.Repositories {
		selectedRepositories[repository.ID] = struct{}{}
	}
	for _, repository := range repositories {
		if _, selected := selectedRepositories[repository.ID]; !selected {
			continue
		}
		config.Repositories = append(config.Repositories, orchestrationmodel.WorkspaceRepository{
			ID: repository.ID, ProviderName: repository.ProviderName, URL: repository.URL,
			IntegrationID: repository.IntegrationID, ProviderRepositoryID: repository.ProviderRepositoryID,
			DefaultBranch: repository.DefaultBranch, Name: repository.Name,
		})
	}
	return d.resolveWorkspaceGitCredentials(ctx, userID, organizationID, value.WorkspaceID, config)
}

func (d *TaskDispatcher) resolveWorkspaceGitCredentials(ctx context.Context, userID, organizationID, workspaceID string, config *orchestrationmodel.RunConfig) error {
	if d.codingCredentials == nil {
		return nil
	}
	integrations, err := d.codingCredentials.ListIntegrations(ctx, userID, organizationID, workspaceID)
	if err != nil {
		return fmt.Errorf("resolve workspace coding integrations: %w", err)
	}
	for _, integration := range integrations {
		credential, err := d.codingCredentials.CodingCredential(ctx, workspaceID, integration.ID)
		if err != nil {
			return fmt.Errorf("resolve workspace coding credential: %w", err)
		}
		config.GitCredentials = append(config.GitCredentials, orchestrationmodel.GitCredential{ProviderName: integration.ProviderName, ProviderConfig: credential.ProviderConfig, RepositoryURL: integration.BaseURL, Token: credential.Token})
	}
	config.GitCredentialsRequired = config.GitCredentialsRequired || len(config.GitCredentials) > 0
	return nil
}

func (d *TaskDispatcher) freshConfiguration(ctx context.Context, userID, organizationID string, value taskmodel.Task) (orchestrationmodel.RunConfig, agentmodel.Agent, error) {
	return d.freshAgentConfiguration(ctx, userID, organizationID, value.WorkspaceID, value.ID, value.AssigneeAgentID)
}

func (d *TaskDispatcher) freshAgentConfiguration(ctx context.Context, userID, organizationID, workspaceID, executionID, agentID string) (orchestrationmodel.RunConfig, agentmodel.Agent, error) {
	definition, err := d.agents.Get(ctx, userID, organizationID, workspaceID, agentID)
	if err != nil {
		return orchestrationmodel.RunConfig{}, agentmodel.Agent{}, fmt.Errorf("get assigned agent: %w", err)
	}
	systemPrompt := strings.TrimSpace(definition.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = strings.TrimSpace(definition.Description)
	}
	if systemPrompt == "" {
		systemPrompt = "Complete the provided task."
	}
	config := orchestrationmodel.RunConfig{
		RuntimeConfig: orchestrationmodel.RuntimeConfig{
			OrganizationID: organizationID,
			WorkspaceID:    workspaceID,
			TaskID:         executionID,

			Runtime: definition.Runtime,
			Agent: orchestrationmodel.AgentDefinition{
				Name:             definition.Name,
				Description:      definition.Description,
				Instructions:     systemPrompt,
				PermissionPolicy: string(definition.PermissionPolicy),
			},
			Model: orchestrationmodel.ModelSelection{Provider: definition.Provider, ID: definition.Model},
		},
	}
	return config, definition, nil
}

func (d *TaskDispatcher) resolveAgentCapabilities(ctx context.Context, organizationID, workspaceID string, definition agentmodel.Agent, config *orchestrationmodel.RunConfig) error {
	if len(definition.SkillIDs) > 0 {
		if d.skillResolver == nil {
			return errors.New("agent skill resolver is unavailable")
		}
		resolved, resolveErr := d.skillResolver.ResolveForAgent(ctx, organizationID, workspaceID, definition.SkillIDs)
		if resolveErr != nil {
			return fmt.Errorf("resolve agent skills: %w", resolveErr)
		}
		for _, item := range resolved {
			config.Skills = append(config.Skills, orchestrationmodel.Skill{ID: item.ID, SkillMD: item.SkillMD})
		}
		if len(config.Skills) != len(definition.SkillIDs) {
			return errors.New("one or more selected agent skills are unavailable")
		}
	}
	if len(definition.MCPServerIDs) > 0 {
		if d.mcpServers == nil {
			return errors.New("agent MCP server resolver is unavailable")
		}
		servers, resolveErr := d.mcpServers.Resolve(ctx, workspaceID, definition.MCPServerIDs)
		if resolveErr != nil {
			return fmt.Errorf("resolve agent MCP servers: %w", resolveErr)
		}
		for _, server := range servers {
			config.MCPServers = append(config.MCPServers, orchestrationmodel.MCPServer{ID: server.ID, Name: server.Name, URL: server.URL, Headers: server.Headers, Transport: server.Transport, Command: server.Command, Args: server.Args, Environment: server.Environment})
		}
	}
	return nil
}
