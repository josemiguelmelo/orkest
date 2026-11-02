package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/agent"
	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	"github.com/josemiguelmelo/orkest/internal/coderepository"
	"github.com/josemiguelmelo/orkest/internal/core/ids"
	coreruntime "github.com/josemiguelmelo/orkest/internal/core/runtime"
	"github.com/josemiguelmelo/orkest/internal/mcpserver"
	"github.com/josemiguelmelo/orkest/internal/skill"
	"github.com/josemiguelmelo/orkest/internal/task"
	"github.com/josemiguelmelo/orkest/internal/workspaceenv"
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
	agents   taskAgentGetter
	profiles interface {
		Resolve(context.Context, string, string, agent.Runtime) (string, error)
	}
	launches   *LaunchService
	mcpServers interface {
		Resolve(context.Context, string, []string) ([]mcpserver.RuntimeServer, error)
	}
	skillResolver interface {
		ResolveForAgent(context.Context, string, string, []string) ([]skill.Skill, error)
	}
	environmentResolver interface {
		Resolve(context.Context, string) (workspaceenv.RuntimeValues, error)
	}
	repositories interface {
		List(context.Context, string, string, string) ([]coderepository.CodeRepository, error)
	}
	agentTools interface {
		RuntimeServer(string, string, string, string) (mcpserver.RuntimeServer, error)
	}
	codingCredentials interface {
		CodingCredential(context.Context, string, string) (coderepository.Credentials, error)
	}
}

func (d *TaskDispatcher) SetAgentTools(tools interface {
	RuntimeServer(string, string, string, string) (mcpserver.RuntimeServer, error)
},
) {
	d.agentTools = tools
}

func (d *TaskDispatcher) SetProfiles(profiles interface {
	Resolve(context.Context, string, string, agent.Runtime) (string, error)
},
) {
	d.profiles = profiles
}

func (d *TaskDispatcher) SetEnvironmentResolver(resolver interface {
	Resolve(context.Context, string) (workspaceenv.RuntimeValues, error)
},
) {
	d.environmentResolver = resolver
}

func (d *TaskDispatcher) SetRepositoryResolver(resolver interface {
	List(context.Context, string, string, string) ([]coderepository.CodeRepository, error)
},
) {
	d.repositories = resolver
}

func (d *TaskDispatcher) SetCodingCredentialResolver(resolver interface {
	CodingCredential(context.Context, string, string) (coderepository.Credentials, error)
},
) {
	d.codingCredentials = resolver
}

func (d *TaskDispatcher) SetSkillResolver(resolver interface {
	ResolveForAgent(context.Context, string, string, []string) ([]skill.Skill, error)
},
) {
	d.skillResolver = resolver
}

func NewTaskDispatcher(agents taskAgentGetter, launches *LaunchService, mcpServers ...interface {
	Resolve(context.Context, string, []string) ([]mcpserver.RuntimeServer, error)
},
) (*TaskDispatcher, error) {
	if agents == nil || launches == nil {
		return nil, errors.New("task dispatch dependencies are required")
	}
	dispatcher := &TaskDispatcher{agents: agents, launches: launches}
	if len(mcpServers) > 0 {
		dispatcher.mcpServers = mcpServers[0]
	}
	return dispatcher, nil
}

func (d *TaskDispatcher) StartTask(ctx context.Context, userID, organizationID string, value task.Task) error {
	if strings.TrimSpace(value.AssigneeAgentID) == "" {
		return errors.New("task has no assigned agent")
	}
	runID, err := ids.NewUUID()
	if err != nil {
		return fmt.Errorf("generate task run ID: %w", err)
	}
	commandID := value.CommandID
	if commandID == "" {
		commandID, err = ids.NewUUID()
		if err != nil {
			return err
		}
	}
	var config runconfig.RunConfig
	var definition agent.Agent
	var continuing bool
	sessions := d.launches.repo
	if value.SessionMode != "new" {
		config, continuing, err = sessions.CurrentSession(ctx, organizationID, value.WorkspaceID, value.ID)
		if err != nil {
			return err
		}
	}
	if !continuing {
		config, definition, err = d.freshConfiguration(ctx, userID, organizationID, value)
		if err != nil {
			return err
		}
		config.TaskSessionID, err = ids.NewUUID()
		if err != nil {
			return err
		}
	}
	if !continuing {
		config.ExecutionProfileID = definition.ExecutionProfileID
	}
	if config.ExecutionProfileID != "" {
		if d.profiles == nil {
			return errors.New("execution profiles unavailable")
		}
		config.ExecutionImage, err = d.profiles.Resolve(ctx, value.WorkspaceID, config.ExecutionProfileID, config.Runtime)
		if err != nil {
			return err
		}
	}
	config.RunID, config.RestoreSession = runID, continuing
	if d.agentTools != nil {
		server, serverErr := d.agentTools.RuntimeServer(userID, organizationID, value.WorkspaceID, value.ID)
		if serverErr != nil {
			return fmt.Errorf("configure Orkest agent tools: %w", serverErr)
		}
		config.MCPServers = append(config.MCPServers, runconfig.MCPServer{ID: server.ID, Name: server.Name, URL: server.URL, Headers: server.Headers})
	}
	config.GitCredentials = nil
	config.GitCredentialsRequired = false
	config.Repositories = nil
	if d.environmentResolver != nil {
		config.Environment, err = d.environmentResolver.Resolve(ctx, value.WorkspaceID)
		if err != nil {
			return fmt.Errorf("resolve workspace environment: %w", err)
		}
	}
	if !continuing {
		if config.Environment == nil {
			config.Environment = make(map[string]string)
		}
		for name, value := range definition.Configuration {
			config.Environment[name] = value
		}
		if err := coreruntime.ValidateRequiredEnvironment(config.Runtime, config.Environment); err != nil {
			return fmt.Errorf("resolve runtime configuration: %w", err)
		}
	}
	if d.repositories != nil {
		repositories, resolveErr := d.repositories.List(ctx, userID, organizationID, value.WorkspaceID)
		if resolveErr != nil {
			return fmt.Errorf("resolve workspace repositories: %w", resolveErr)
		}
		selectedRepositories := make(map[string]struct{}, len(value.Repositories))
		for _, repository := range value.Repositories {
			selectedRepositories[repository.ID] = struct{}{}
		}
		for _, repository := range repositories {
			if _, selected := selectedRepositories[repository.ID]; !selected {
				continue
			}
			config.Repositories = append(config.Repositories, runconfig.WorkspaceRepository{
				ID: repository.ID, ProviderName: repository.ProviderName, URL: repository.URL,
				IntegrationID: repository.IntegrationID, ProviderRepositoryID: repository.ProviderRepositoryID,
				DefaultBranch: repository.DefaultBranch, Name: repository.Name,
			})
			if repository.IntegrationID != "" {
				if d.codingCredentials == nil {
					return errors.New("coding credential resolver is unavailable")
				}
				credential, credentialErr := d.codingCredentials.CodingCredential(ctx, value.WorkspaceID, repository.IntegrationID)
				if credentialErr != nil {
					return fmt.Errorf("resolve repository push credential: %w", credentialErr)
				}
				config.GitCredentials = append(config.GitCredentials, runconfig.GitCredential{ProviderName: repository.ProviderName, ProviderConfig: credential.ProviderConfig, RepositoryURL: repository.URL, RepositoryPath: "repositories/" + repository.ProviderName + "/" + repository.ID, Token: credential.Token})
				config.GitCredentialsRequired = true
			}
		}
	}
	if !continuing {
		if len(definition.SkillIDs) > 0 {
			if d.skillResolver == nil {
				return errors.New("agent skill resolver is unavailable")
			}
			resolved, resolveErr := d.skillResolver.ResolveForAgent(ctx, organizationID, value.WorkspaceID, definition.SkillIDs)
			if resolveErr != nil {
				return fmt.Errorf("resolve agent skills: %w", resolveErr)
			}
			for _, item := range resolved {
				config.Skills = append(config.Skills, runconfig.Skill{ID: item.ID, SkillMD: item.SkillMD})
			}
			if len(config.Skills) != len(definition.SkillIDs) {
				return errors.New("one or more selected agent skills are unavailable")
			}
		}
		if len(definition.MCPServerIDs) > 0 {
			if d.mcpServers == nil {
				return errors.New("agent MCP server resolver is unavailable")
			}
			servers, resolveErr := d.mcpServers.Resolve(ctx, value.WorkspaceID, definition.MCPServerIDs)
			if resolveErr != nil {
				return fmt.Errorf("resolve agent MCP servers: %w", resolveErr)
			}
			for _, server := range servers {
				config.MCPServers = append(config.MCPServers, runconfig.MCPServer{ID: server.ID, Name: server.Name, URL: server.URL, Headers: server.Headers})
			}
		}
	} else if len(config.MCPServers) > 0 {
		if d.mcpServers == nil {
			return errors.New("agent MCP server resolver is unavailable")
		}
		serverIDs := make([]string, 0, len(config.MCPServers))
		for _, server := range config.MCPServers {
			if server.ID != runconfig.OrkestMCPServerID {
				serverIDs = append(serverIDs, server.ID)
			}
		}
		servers, err := d.mcpServers.Resolve(ctx, value.WorkspaceID, serverIDs)
		if err != nil {
			return err
		}
		if len(servers) != len(serverIDs) {
			return errors.New("session MCP server is unavailable")
		}
		for i := range config.MCPServers {
			for _, server := range servers {
				if config.MCPServers[i].ID == server.ID {
					config.MCPServers[i].Headers = server.Headers
				}
			}
		}
	}
	prompt, err := taskPrompt(value, config.Repositories)
	if err != nil {
		return err
	}
	if value.PromptText != "" {
		prompt = value.PromptText
	}
	if err := config.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(commandID) == "" || len(commandID) > 200 || strings.TrimSpace(prompt) == "" || len(prompt) > interaction.MaxPromptBytes {
		return interaction.ErrCommand
	}
	config.InitialCommandID, config.InitialPrompt = commandID, prompt
	if err := sessions.ReserveSessionRun(ctx, config, value.StageVersion); err != nil {
		return err
	}
	config.ReservedRun = true
	if _, err = d.launches.StartWithPrompt(ctx, config, commandID, prompt); err != nil {
		return fmt.Errorf("start agent run: %w", err)
	}
	return nil
}

func taskPrompt(value task.Task, repositories []runconfig.WorkspaceRepository) (string, error) {
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

func (d *TaskDispatcher) freshConfiguration(ctx context.Context, userID, organizationID string, value task.Task) (runconfig.RunConfig, agent.Agent, error) {
	definition, err := d.agents.Get(ctx, userID, organizationID, value.WorkspaceID, value.AssigneeAgentID)
	if err != nil {
		return runconfig.RunConfig{}, agent.Agent{}, fmt.Errorf("get assigned agent: %w", err)
	}
	systemPrompt := strings.TrimSpace(definition.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = strings.TrimSpace(definition.Description)
	}
	if systemPrompt == "" {
		systemPrompt = "Complete the provided task."
	}
	config := runconfig.RunConfig{
		RuntimeConfig: runconfig.RuntimeConfig{
			OrganizationID: organizationID,
			WorkspaceID:    value.WorkspaceID,
			TaskID:         value.ID,

			Runtime: definition.Runtime,
			Agent: runconfig.AgentDefinition{
				Name:             definition.Name,
				Description:      definition.Description,
				Instructions:     systemPrompt,
				PermissionPolicy: string(definition.PermissionPolicy),
			},
			Model: runconfig.ModelSelection{Provider: definition.Provider, ID: definition.Model},
		},
	}
	return config, definition, nil
}

func (d *TaskDispatcher) TaskCommandAccepted(ctx context.Context, orgID, workspaceID, taskID, commandID, prompt string) (bool, error) {
	return d.launches.repo.TaskCommand(ctx, orgID, workspaceID, taskID, commandID, prompt)
}
