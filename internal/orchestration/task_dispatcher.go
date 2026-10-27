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
	"github.com/josemiguelmelo/orkest/internal/mcpserver"
	"github.com/josemiguelmelo/orkest/internal/skill"
	"github.com/josemiguelmelo/orkest/internal/task"
	"github.com/josemiguelmelo/orkest/internal/workspaceenv"
)

type taskAgentGetter interface {
	Get(context.Context, string, string, string, string) (agent.Agent, error)
}

// TaskDispatcher resolves an assigned agent definition and starts its task run.
type TaskDispatcher struct {
	agents     taskAgentGetter
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
	config.RunID, config.RestoreSession = runID, continuing
	if d.environmentResolver != nil {
		config.Environment, err = d.environmentResolver.Resolve(ctx, value.WorkspaceID)
		if err != nil {
			return fmt.Errorf("resolve workspace environment: %w", err)
		}
	}
	if d.repositories != nil {
		repositories, resolveErr := d.repositories.List(ctx, userID, organizationID, value.WorkspaceID)
		if resolveErr != nil {
			return fmt.Errorf("resolve workspace repositories: %w", resolveErr)
		}
		for _, repository := range repositories {
			config.Repositories = append(config.Repositories, runconfig.WorkspaceRepository{
				ID: repository.ID, ProviderName: repository.ProviderName, URL: repository.URL,
			})
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
			serverIDs = append(serverIDs, server.ID)
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
	if err := sessions.ReserveSessionRun(ctx, config); err != nil {
		return err
	}
	config.ReservedRun = true
	if _, err = d.launches.StartWithPrompt(ctx, config, commandID, prompt); err != nil {
		return fmt.Errorf("start agent run: %w", err)
	}
	return nil
}

func taskPrompt(value task.Task, repositories []runconfig.WorkspaceRepository) (string, error) {
	snapshot := struct {
		Title        string           `json:"title"`
		Description  string           `json:"description"`
		Labels       []string         `json:"labels"`
		Components   []task.Component `json:"components"`
		Repositories []struct {
			ID   string `json:"id"`
			Path string `json:"path"`
		} `json:"repositories,omitempty"`
	}{Title: value.Title, Description: value.Description, Labels: value.Labels, Components: value.Components}
	for _, repository := range repositories {
		snapshot.Repositories = append(snapshot.Repositories, struct {
			ID   string `json:"id"`
			Path string `json:"path"`
		}{ID: repository.ID, Path: "repositories/" + repository.ProviderName + "/" + repository.ID})
	}
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode task context: %w", err)
	}
	return "Work on the following Orkest task. Use this task snapshot as the source of truth for the requested work. Workspace repositories are checked out under the listed paths on task-specific branches; inspect the repositories relevant to the task and make changes in those checkouts:\n\n" + string(encoded), nil
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
				Name:         definition.Name,
				Description:  definition.Description,
				Instructions: systemPrompt,
			},
			Model: runconfig.ModelSelection{Provider: definition.Provider, ID: definition.Model},
		},
	}
	return config, definition, nil
}

func (d *TaskDispatcher) TaskCommandAccepted(ctx context.Context, orgID, workspaceID, taskID, commandID, prompt string) (bool, error) {
	return d.launches.repo.TaskCommand(ctx, orgID, workspaceID, taskID, commandID, prompt)
}
