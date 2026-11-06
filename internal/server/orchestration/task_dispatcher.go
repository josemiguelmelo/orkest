package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	"github.com/josemiguelmelo/orkest/internal/server/agent"
	"github.com/josemiguelmelo/orkest/internal/server/artifact"
	"github.com/josemiguelmelo/orkest/internal/server/coderepository"
	"github.com/josemiguelmelo/orkest/internal/server/mcpserver"
	"github.com/josemiguelmelo/orkest/internal/server/skill"
	"github.com/josemiguelmelo/orkest/internal/server/task"
	"github.com/josemiguelmelo/orkest/internal/server/workflow"
	"github.com/josemiguelmelo/orkest/internal/server/workspaceenv"
)

type taskAgentGetter interface {
	Get(context.Context, string, string, string, string) (agent.Agent, error)
}

type taskPromptRepository struct {
	ID           string `json:"id"`
	ProviderName string `json:"providerName"`
	Name         string `json:"name"`
	URL          string `json:"url"`
	Path         string `json:"path"`
}

type taskPromptSnapshot struct {
	StageName        string                 `json:"stageName,omitempty"`
	StageDescription string                 `json:"stageDescription,omitempty"`
	ID               string                 `json:"id"`
	PlatformTaskID   string                 `json:"platformTaskId"`
	SourceProvider   string                 `json:"sourceProvider,omitempty"`
	SourceID         string                 `json:"sourceId,omitempty"`
	Title            string                 `json:"title"`
	Description      string                 `json:"description"`
	Labels           []string               `json:"labels"`
	Repositories     []taskPromptRepository `json:"repositories"`
}

// TaskDispatcher resolves an assigned agent definition and starts its task run.
type TaskDispatcher struct {
	artifacts           TaskArtifactReader
	workflows           TaskWorkflowReader
	agents              taskAgentGetter
	profiles            ExecutionImageResolver
	launches            *LaunchService
	mcpServers          TaskMCPResolver
	skillResolver       TaskSkillResolver
	environmentResolver TaskEnvironmentResolver
	repositories        TaskRepositoryReader
	agentTools          TaskToolResolver
	codingCredentials   TaskCredentialResolver
}
type (
	TaskArtifactReader interface {
		List(context.Context, artifact.Scope) ([]artifact.Revision, error)
	}
	TaskWorkflowReader interface {
		Get(context.Context, string, string, string, string) (workflow.Definition, error)
	}
	ExecutionImageResolver interface {
		Resolve(context.Context, string, string, agent.Runtime) (string, error)
	}
	TaskMCPResolver interface {
		Resolve(context.Context, string, []string) ([]mcpserver.RuntimeServer, error)
	}
	TaskSkillResolver interface {
		ResolveForAgent(context.Context, string, string, []string) ([]skill.Skill, error)
	}
	TaskEnvironmentResolver interface {
		Resolve(context.Context, string) (workspaceenv.RuntimeValues, error)
	}
	TaskRepositoryReader interface {
		List(context.Context, string, string, string) ([]coderepository.CodeRepository, error)
	}
	TaskToolResolver interface {
		RuntimeServer(string, string, string, string) (mcpserver.RuntimeServer, error)
	}
	TaskCredentialResolver interface {
		CodingCredential(context.Context, string, string) (coderepository.Credentials, error)
	}
)

func NewTaskDispatcher(agents taskAgentGetter, launches *LaunchService, mcpServers TaskMCPResolver, profiles ExecutionImageResolver, skills TaskSkillResolver, environment TaskEnvironmentResolver, repositories TaskRepositoryReader, credentials TaskCredentialResolver, tools TaskToolResolver, artifacts TaskArtifactReader, workflows TaskWorkflowReader) (*TaskDispatcher, error) {
	if agents == nil || launches == nil || (artifacts != nil && workflows == nil) {
		return nil, errors.New("task dispatch dependencies are required")
	}
	return &TaskDispatcher{agents: agents, launches: launches, mcpServers: mcpServers, profiles: profiles, skillResolver: skills, environmentResolver: environment, repositories: repositories, codingCredentials: credentials, agentTools: tools, artifacts: artifacts, workflows: workflows}, nil
}

func (d *TaskDispatcher) StartTask(ctx context.Context, userID, organizationID string, command task.StartCommand) error {
	value := command.Task
	if strings.TrimSpace(value.AssigneeAgentID) == "" {
		return errors.New("task has no assigned agent")
	}
	runID, err := ids.NewUUID()
	if err != nil {
		return fmt.Errorf("generate task run ID: %w", err)
	}
	commandID := command.CommandID
	if commandID == "" {
		commandID, err = ids.NewUUID()
		if err != nil {
			return err
		}
	}
	sessions := d.launches.repo
	var config RunConfig
	var continuing bool
	if command.SessionMode != "new" {
		config, continuing, err = d.continueTaskSession(ctx, userID, organizationID, value, runID)
		if err != nil {
			return err
		}
	}
	if !continuing {
		config, err = d.createTaskSession(ctx, userID, organizationID, value, runID)
		if err != nil {
			return err
		}
	}
	prompt, err := taskPrompt(value, config.Repositories)
	if err != nil {
		return err
	}
	if command.PromptText != "" {
		prompt = command.PromptText
	}
	if err = d.refreshOrkestMCPServer(userID, organizationID, value, &config); err != nil {
		return err
	}
	prompt, err = d.artifactPrompt(ctx, userID, organizationID, value, &config, prompt)
	if err != nil {
		return err
	}
	if err := config.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(commandID) == "" || len(commandID) > 200 || strings.TrimSpace(prompt) == "" || len(prompt) > MaxPromptBytes {
		return ErrCommand
	}
	config.InitialCommandID, config.InitialPrompt = commandID, prompt
	config.DeliveryToken = command.DeliveryToken
	if err := sessions.ReserveSessionRun(ctx, config, value.StageVersion); err != nil {
		return err
	}
	config.ReservedRun = true
	if _, err = d.launches.StartWithPrompt(ctx, config, commandID, prompt); err != nil {
		return fmt.Errorf("start agent run: %w", err)
	}
	return nil
}

func (d *TaskDispatcher) createTaskSession(ctx context.Context, userID, organizationID string, value task.Task, runID string) (RunConfig, error) {
	config, definition, err := d.freshConfiguration(ctx, userID, organizationID, value)
	if err != nil {
		return RunConfig{}, err
	}
	config.TaskSessionID, err = ids.NewUUID()
	if err != nil {
		return RunConfig{}, err
	}
	config.ExecutionProfileID = definition.ExecutionProfileID
	if err = d.resolveExecutionImage(ctx, value.WorkspaceID, &config); err != nil {
		return RunConfig{}, err
	}
	config.RunID, config.RestoreSession = runID, false
	config.GitCredentials = nil
	config.GitCredentialsRequired = false
	config.Repositories = nil

	if d.environmentResolver != nil {
		config.Environment, err = d.environmentResolver.Resolve(ctx, value.WorkspaceID)
		if err != nil {
			return RunConfig{}, fmt.Errorf("resolve workspace environment: %w", err)
		}
	}
	if config.Environment == nil {
		config.Environment = make(map[string]string)
	}
	for name, value := range definition.Configuration {
		config.Environment[name] = value
	}
	if err = coreruntime.ValidateRequiredEnvironment(config.Runtime, config.Environment); err != nil {
		return RunConfig{}, fmt.Errorf("resolve runtime configuration: %w", err)
	}
	if err = d.resolveTaskRepositories(ctx, userID, organizationID, value, &config); err != nil {
		return RunConfig{}, err
	}
	if len(definition.SkillIDs) > 0 {
		if d.skillResolver == nil {
			return RunConfig{}, errors.New("agent skill resolver is unavailable")
		}
		resolved, resolveErr := d.skillResolver.ResolveForAgent(ctx, organizationID, value.WorkspaceID, definition.SkillIDs)
		if resolveErr != nil {
			return RunConfig{}, fmt.Errorf("resolve agent skills: %w", resolveErr)
		}
		for _, item := range resolved {
			config.Skills = append(config.Skills, Skill{ID: item.ID, SkillMD: item.SkillMD})
		}
		if len(config.Skills) != len(definition.SkillIDs) {
			return RunConfig{}, errors.New("one or more selected agent skills are unavailable")
		}
	}
	if len(definition.MCPServerIDs) > 0 {
		if d.mcpServers == nil {
			return RunConfig{}, errors.New("agent MCP server resolver is unavailable")
		}
		servers, resolveErr := d.mcpServers.Resolve(ctx, value.WorkspaceID, definition.MCPServerIDs)
		if resolveErr != nil {
			return RunConfig{}, fmt.Errorf("resolve agent MCP servers: %w", resolveErr)
		}
		for _, server := range servers {
			config.MCPServers = append(config.MCPServers, MCPServer{ID: server.ID, Name: server.Name, URL: server.URL, Headers: server.Headers})
		}
	}
	if err = d.refreshOrkestMCPServer(userID, organizationID, value, &config); err != nil {
		return RunConfig{}, err
	}
	return config, nil
}

func (d *TaskDispatcher) continueTaskSession(ctx context.Context, userID, organizationID string, value task.Task, runID string) (RunConfig, bool, error) {
	config, found, err := d.launches.repo.CurrentSession(ctx, organizationID, value.WorkspaceID, value.ID)
	if err != nil || !found {
		return config, found, err
	}
	if err = d.resolveExecutionImage(ctx, value.WorkspaceID, &config); err != nil {
		return RunConfig{}, false, err
	}
	config.RunID, config.RestoreSession = runID, true
	config.GitCredentials = nil
	config.GitCredentialsRequired = false
	config.Repositories = nil
	if d.environmentResolver != nil {
		environment, resolveErr := d.environmentResolver.Resolve(ctx, value.WorkspaceID)
		if resolveErr != nil {
			return RunConfig{}, false, fmt.Errorf("resolve workspace environment: %w", resolveErr)
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
		return RunConfig{}, false, err
	}
	if len(config.MCPServers) > 0 {
		serverIDs := make([]string, 0, len(config.MCPServers))
		for _, server := range config.MCPServers {
			if server.ID != OrkestMCPServerID {
				serverIDs = append(serverIDs, server.ID)
			}
		}
		if len(serverIDs) > 0 {
			if d.mcpServers == nil {
				return RunConfig{}, false, errors.New("session MCP server resolver is unavailable")
			}
			servers, resolveErr := d.mcpServers.Resolve(ctx, value.WorkspaceID, serverIDs)
			if resolveErr != nil {
				return RunConfig{}, false, fmt.Errorf("resolve session MCP server credentials: %w", resolveErr)
			}
			if len(servers) != len(serverIDs) {
				return RunConfig{}, false, errors.New("session MCP server is unavailable")
			}
			for i := range config.MCPServers {
				for _, server := range servers {
					if config.MCPServers[i].ID == server.ID {
						config.MCPServers[i].Headers = server.Headers
					}
				}
			}
		}
	}
	if err = d.refreshOrkestMCPServer(userID, organizationID, value, &config); err != nil {
		return RunConfig{}, false, err
	}
	return config, true, nil
}

func (d *TaskDispatcher) resolveExecutionImage(ctx context.Context, workspaceID string, config *RunConfig) error {
	if config.ExecutionProfileID == "" {
		return nil
	}
	if d.profiles == nil {
		return errors.New("execution profiles unavailable")
	}
	image, err := d.profiles.Resolve(ctx, workspaceID, config.ExecutionProfileID, config.Runtime)
	if err != nil {
		return fmt.Errorf("resolve execution profile: %w", err)
	}
	config.ExecutionImage = image
	return nil
}

func (d *TaskDispatcher) refreshOrkestMCPServer(userID, organizationID string, value task.Task, config *RunConfig) error {
	if d.agentTools == nil {
		return nil
	}
	server, err := d.agentTools.RuntimeServer(userID, organizationID, value.WorkspaceID, value.ID)
	if scoped, ok := d.agentTools.(interface {
		RuntimeServerForRun(string, string, string, string, string) (mcpserver.RuntimeServer, error)
	}); ok {
		server, err = scoped.RuntimeServerForRun(userID, organizationID, value.WorkspaceID, value.ID, config.RunID)
	}
	if err != nil {
		return fmt.Errorf("configure Orkest agent tools: %w", err)
	}
	servers := make([]MCPServer, 0, len(config.MCPServers)+1)
	found := false
	for _, existing := range config.MCPServers {
		if existing.ID == server.ID {
			if !found {
				servers = append(servers, MCPServer{ID: server.ID, Name: server.Name, URL: server.URL, Headers: server.Headers})
				found = true
			}
			continue
		}
		servers = append(servers, existing)
	}
	if !found {
		servers = append(servers, MCPServer{ID: server.ID, Name: server.Name, URL: server.URL, Headers: server.Headers})
	}
	config.MCPServers = servers
	return nil
}

func (d *TaskDispatcher) resolveTaskRepositories(ctx context.Context, userID, organizationID string, value task.Task, config *RunConfig) error {
	config.Repositories = nil
	config.GitCredentials = nil
	config.GitCredentialsRequired = false
	if d.repositories == nil {
		return nil
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
		config.Repositories = append(config.Repositories, WorkspaceRepository{
			ID: repository.ID, ProviderName: repository.ProviderName, URL: repository.URL,
			IntegrationID: repository.IntegrationID, ProviderRepositoryID: repository.ProviderRepositoryID,
			DefaultBranch: repository.DefaultBranch, Name: repository.Name,
		})
		if repository.IntegrationID == "" {
			continue
		}
		if d.codingCredentials == nil {
			return errors.New("coding credential resolver is unavailable")
		}
		credential, err := d.codingCredentials.CodingCredential(ctx, value.WorkspaceID, repository.IntegrationID)
		if err != nil {
			return fmt.Errorf("resolve repository push credential: %w", err)
		}
		config.GitCredentials = append(config.GitCredentials, GitCredential{ProviderName: repository.ProviderName, ProviderConfig: credential.ProviderConfig, RepositoryURL: repository.URL, RepositoryPath: "repositories/" + repository.ProviderName + "/" + repository.ID, Token: credential.Token})
		config.GitCredentialsRequired = true
	}
	return nil
}

func taskPrompt(value task.Task, repositories []WorkspaceRepository) (string, error) {
	taskID := value.ID
	if strings.TrimSpace(value.SourceID) != "" {
		taskID = value.SourceID
	}
	snapshot := taskPromptSnapshot{StageName: value.StageName, StageDescription: value.StageDescription, ID: taskID, PlatformTaskID: value.ID, SourceProvider: value.SourceProvider, SourceID: value.SourceID, Title: value.Title, Description: value.Description, Labels: value.Labels}
	for _, repository := range repositories {
		snapshot.Repositories = append(snapshot.Repositories, taskPromptRepository{ID: repository.ID, ProviderName: repository.ProviderName, Name: repository.Name, URL: repository.URL, Path: "repositories/" + repository.ProviderName + "/" + repository.ID})
	}
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode task context: %w", err)
	}
	return "Work on the following task within the scope of its current workflow stage, when provided. Use its external source reference as the main task ID when present; the Orkest platform ID is included for traceability. The selected repositories are checked out under the listed paths on their current default or preserved branches. Inspect them and create or switch to a task-specific branch only when the requested changes require one. Reuse an existing task branch for review iterations:\n\n" + string(encoded), nil
}

func (d *TaskDispatcher) freshConfiguration(ctx context.Context, userID, organizationID string, value task.Task) (RunConfig, agent.Agent, error) {
	definition, err := d.agents.Get(ctx, userID, organizationID, value.WorkspaceID, value.AssigneeAgentID)
	if err != nil {
		return RunConfig{}, agent.Agent{}, fmt.Errorf("get assigned agent: %w", err)
	}
	systemPrompt := strings.TrimSpace(definition.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = strings.TrimSpace(definition.Description)
	}
	if systemPrompt == "" {
		systemPrompt = "Complete the provided task."
	}
	config := RunConfig{
		RuntimeConfig: RuntimeConfig{
			OrganizationID: organizationID,
			WorkspaceID:    value.WorkspaceID,
			TaskID:         value.ID,

			Runtime: definition.Runtime,
			Agent: AgentDefinition{
				Name:             definition.Name,
				Description:      definition.Description,
				Instructions:     systemPrompt,
				PermissionPolicy: string(definition.PermissionPolicy),
			},
			Model: ModelSelection{Provider: definition.Provider, ID: definition.Model},
		},
	}
	return config, definition, nil
}

func (d *TaskDispatcher) TaskCommandAccepted(ctx context.Context, orgID, workspaceID, taskID, commandID, prompt string) (bool, error) {
	return d.launches.repo.TaskCommand(ctx, orgID, workspaceID, taskID, commandID, prompt)
}
