package orchestration

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	"github.com/josemiguelmelo/orkest/internal/server/agent"
	"github.com/josemiguelmelo/orkest/internal/server/task"
)

type DirectConversation struct {
	ID        string `json:"id"`
	AgentID   string `json:"agentId"`
	CreatedAt string `json:"createdAt"`
}

type DirectConversationStore interface {
	CreateDirectConversation(context.Context, string, string, string, string) (DirectConversation, error)
	GetDirectConversation(context.Context, string, string, string) (DirectConversation, error)
	ListDirectConversations(context.Context, string, string) ([]DirectConversation, error)
	DeleteDirectConversation(context.Context, string, string, string) error
}

// DirectChatService creates workspace-scoped agent conversations and routes
// messages through the same durable session and launch services as task chat.
type DirectChatService struct {
	store      DirectConversationStore
	dispatcher *TaskDispatcher
	launches   *LaunchService
}

func NewDirectChatService(store DirectConversationStore, dispatcher *TaskDispatcher, launches *LaunchService) (*DirectChatService, error) {
	if store == nil || dispatcher == nil || launches == nil {
		return nil, errors.New("direct chat dependencies are required")
	}
	return &DirectChatService{store: store, dispatcher: dispatcher, launches: launches}, nil
}

func (s *DirectChatService) List(ctx context.Context, orgID, workspaceID string) ([]DirectConversation, error) {
	return s.store.ListDirectConversations(ctx, orgID, workspaceID)
}

func (s *DirectChatService) Create(ctx context.Context, userID, orgID, workspaceID, agentID string) (DirectConversation, error) {
	conversationID, err := ids.NewUUID()
	if err != nil {
		return DirectConversation{}, fmt.Errorf("generate conversation ID: %w", err)
	}
	conversation, err := s.store.CreateDirectConversation(ctx, conversationID, orgID, workspaceID, agentID)
	if err != nil {
		return DirectConversation{}, err
	}
	if err := s.dispatcher.StartDirectChat(ctx, userID, orgID, workspaceID, agentID, conversation.ID, "", ""); err != nil {
		return conversation, fmt.Errorf("start direct agent conversation: %w", err)
	}
	return conversation, nil
}

func (s *DirectChatService) Get(ctx context.Context, orgID, workspaceID, conversationID string) (DirectConversation, error) {
	return s.store.GetDirectConversation(ctx, orgID, workspaceID, conversationID)
}

func (s *DirectChatService) Delete(ctx context.Context, orgID, workspaceID, conversationID string) error {
	return s.store.DeleteDirectConversation(ctx, orgID, workspaceID, conversationID)
}

func (s *DirectChatService) Runs(ctx context.Context, orgID, workspaceID, conversationID string) ([]Launch, error) {
	if _, err := s.Get(ctx, orgID, workspaceID, conversationID); err != nil {
		return nil, err
	}
	return s.launches.TaskLaunches(ctx, orgID, workspaceID, conversationID)
}

func (s *DirectChatService) Events(ctx context.Context, orgID, workspaceID, conversationID, runID string, after uint64) ([]Event, error) {
	if _, err := s.Get(ctx, orgID, workspaceID, conversationID); err != nil {
		return nil, err
	}
	return s.launches.TaskEvents(ctx, orgID, workspaceID, conversationID, runID, after)
}

func (s *DirectChatService) Prompt(ctx context.Context, userID, orgID, workspaceID, conversationID, runID, commandID, prompt string) error {
	conversation, err := s.Get(ctx, orgID, workspaceID, conversationID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(prompt) == "" {
		return errors.New("prompt is required")
	}
	runs, err := s.launches.TaskLaunches(ctx, orgID, workspaceID, conversationID)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if run.RunID == runID && run.Status == LaunchReady {
			return s.launches.SendTaskPrompt(ctx, orgID, workspaceID, conversationID, runID, commandID, prompt)
		}
	}
	found := false
	for _, run := range runs {
		if run.RunID == runID {
			found = true
			break
		}
	}
	if !found {
		return ErrTaskRunNotFound
	}
	return s.dispatcher.StartDirectChat(ctx, userID, orgID, workspaceID, conversation.AgentID, conversationID, commandID, prompt)
}

func (s *DirectChatService) Cancel(ctx context.Context, orgID, workspaceID, conversationID, runID, commandID string) error {
	if _, err := s.Get(ctx, orgID, workspaceID, conversationID); err != nil {
		return err
	}
	return s.launches.CancelTaskPrompt(ctx, orgID, workspaceID, conversationID, runID, commandID)
}

func (s *DirectChatService) RespondPermission(ctx context.Context, orgID, workspaceID, conversationID, runID string, response PermissionResponse) error {
	if _, err := s.Get(ctx, orgID, workspaceID, conversationID); err != nil {
		return err
	}
	return s.launches.RespondTaskPermission(ctx, orgID, workspaceID, conversationID, runID, response)
}

func (s *DirectChatService) Queue(ctx context.Context, orgID, workspaceID, conversationID, runID string, discard bool) error {
	if _, err := s.Get(ctx, orgID, workspaceID, conversationID); err != nil {
		return err
	}
	return s.launches.TaskQueue(ctx, orgID, workspaceID, conversationID, runID, discard)
}

func (d *TaskDispatcher) StartDirectChat(ctx context.Context, userID, organizationID, workspaceID, agentID, conversationID, commandID, prompt string) error {
	sessions, ok := d.launches.repo.(directSessionRepository)
	if !ok {
		return errors.New("direct session storage unavailable")
	}
	runID, err := ids.NewUUID()
	if err != nil {
		return fmt.Errorf("generate direct chat run ID: %w", err)
	}
	config, continuing, err := sessions.CurrentDirectSession(ctx, organizationID, workspaceID, conversationID)
	if err != nil {
		return err
	}
	var definition agent.Agent
	if !continuing {
		value := task.Task{ID: conversationID, WorkspaceID: workspaceID, AssigneeAgentID: agentID}
		config, definition, err = d.freshConfiguration(ctx, userID, organizationID, value)
		if err != nil {
			return err
		}
		config.ExecutionProfileID = definition.ExecutionProfileID
		if strings.TrimSpace(definition.SystemPrompt) == "" && strings.TrimSpace(definition.Description) == "" {
			config.Agent.Instructions = "Assist the user with their requests."
		}
		config.TaskSessionID, err = ids.NewUUID()
		if err != nil {
			return err
		}
		if len(definition.SkillIDs) > 0 {
			if d.skillResolver == nil {
				return errors.New("agent skill resolver is unavailable")
			}
			resolved, resolveErr := d.skillResolver.ResolveForAgent(ctx, organizationID, workspaceID, definition.SkillIDs)
			if resolveErr != nil {
				return fmt.Errorf("resolve agent skills: %w", resolveErr)
			}
			for _, item := range resolved {
				config.Skills = append(config.Skills, Skill{ID: item.ID, SkillMD: item.SkillMD})
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
				config.MCPServers = append(config.MCPServers, MCPServer{ID: server.ID, Name: server.Name, URL: server.URL, Headers: server.Headers})
			}
		}
		if config.ExecutionProfileID != "" {
			if d.profiles == nil {
				return errors.New("execution profiles unavailable")
			}
			config.ExecutionImage, err = d.profiles.Resolve(ctx, workspaceID, config.ExecutionProfileID, config.Runtime)
			if err != nil {
				return err
			}
		}
	} else if len(config.MCPServers) > 0 {
		if d.mcpServers == nil {
			return errors.New("agent MCP server resolver is unavailable")
		}
		serverIDs := make([]string, 0, len(config.MCPServers))
		for _, server := range config.MCPServers {
			if server.ID != OrkestMCPServerID {
				serverIDs = append(serverIDs, server.ID)
			}
		}
		servers, resolveErr := d.mcpServers.Resolve(ctx, workspaceID, serverIDs)
		if resolveErr != nil {
			return resolveErr
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
	if d.environmentResolver != nil {
		config.Environment, err = d.environmentResolver.Resolve(ctx, workspaceID)
		if err != nil {
			return fmt.Errorf("resolve workspace environment: %w", err)
		}
	}
	config.RunID, config.RestoreSession = runID, continuing
	if d.agentTools != nil {
		server, serverErr := d.agentTools.RuntimeServer(userID, organizationID, workspaceID, conversationID)
		if serverErr != nil {
			return fmt.Errorf("configure Orkest agent tools: %w", serverErr)
		}
		found := false
		for i := range config.MCPServers {
			if config.MCPServers[i].ID == server.ID {
				config.MCPServers[i] = MCPServer{ID: server.ID, Name: server.Name, URL: server.URL, Headers: server.Headers}
				found = true
			}
		}
		if !found {
			config.MCPServers = append(config.MCPServers, MCPServer{ID: server.ID, Name: server.Name, URL: server.URL, Headers: server.Headers})
		}
	}
	config.InitialCommandID, config.InitialPrompt = commandID, prompt
	if err := config.Validate(); err != nil {
		return err
	}
	if continuing && strings.TrimSpace(prompt) != "" {
		if strings.TrimSpace(commandID) == "" {
			commandID, err = ids.NewUUID()
			if err != nil {
				return err
			}
			config.InitialCommandID = commandID
		}
		if err = sessions.ReserveDirectSessionRun(ctx, config); err != nil {
			return err
		}
		config.ReservedRun = true
		_, err = d.launches.StartWithPrompt(ctx, config, commandID, prompt)
		return err
	}
	if err = sessions.ReserveDirectSessionRun(ctx, config); err != nil {
		return err
	}
	config.ReservedRun = true
	_, err = d.launches.Start(ctx, config)
	return err
}
