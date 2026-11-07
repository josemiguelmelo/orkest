package orchestration

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

type DirectConversationStore interface {
	CreateDirectConversation(context.Context, string, string, string, string) (orchestrationmodel.DirectConversation, error)
	GetDirectConversation(context.Context, string, string, string) (orchestrationmodel.DirectConversation, error)
	ListDirectConversations(context.Context, string, string) ([]orchestrationmodel.DirectConversation, error)
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

func (s *DirectChatService) List(ctx context.Context, orgID, workspaceID string) ([]orchestrationmodel.DirectConversation, error) {
	return s.store.ListDirectConversations(ctx, orgID, workspaceID)
}

func (s *DirectChatService) Create(ctx context.Context, userID, orgID, workspaceID, agentID string) (orchestrationmodel.DirectConversation, error) {
	conversationID, err := ids.NewUUID()
	if err != nil {
		return orchestrationmodel.DirectConversation{}, fmt.Errorf("generate conversation ID: %w", err)
	}
	conversation, err := s.store.CreateDirectConversation(ctx, conversationID, orgID, workspaceID, agentID)
	if err != nil {
		return orchestrationmodel.DirectConversation{}, err
	}
	if err := s.dispatcher.StartDirectChat(ctx, userID, orgID, workspaceID, agentID, conversation.ID, "", ""); err != nil {
		return conversation, fmt.Errorf("start direct agent conversation: %w", err)
	}
	return conversation, nil
}

func (s *DirectChatService) Get(ctx context.Context, orgID, workspaceID, conversationID string) (orchestrationmodel.DirectConversation, error) {
	return s.store.GetDirectConversation(ctx, orgID, workspaceID, conversationID)
}

func (s *DirectChatService) Delete(ctx context.Context, orgID, workspaceID, conversationID string) error {
	return s.store.DeleteDirectConversation(ctx, orgID, workspaceID, conversationID)
}

func (s *DirectChatService) Runs(ctx context.Context, orgID, workspaceID, conversationID string) ([]orchestrationmodel.Launch, error) {
	if _, err := s.Get(ctx, orgID, workspaceID, conversationID); err != nil {
		return nil, err
	}
	return s.launches.TaskLaunches(ctx, orgID, workspaceID, conversationID)
}

func (s *DirectChatService) Events(ctx context.Context, orgID, workspaceID, conversationID, runID string, after uint64) ([]orchestrationmodel.Event, error) {
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
	run, found := findLaunch(runs, runID)
	if !found {
		return ErrTaskRunNotFound
	}
	if run.Status == orchestrationmodel.LaunchReady {
		return s.launches.SendTaskPrompt(ctx, orgID, workspaceID, conversationID, runID, commandID, prompt)
	}

	return s.dispatcher.StartDirectChat(ctx, userID, orgID, workspaceID, conversation.AgentID, conversationID, commandID, prompt)
}

func (s *DirectChatService) Cancel(ctx context.Context, orgID, workspaceID, conversationID, runID, commandID string) error {
	if _, err := s.Get(ctx, orgID, workspaceID, conversationID); err != nil {
		return err
	}
	return s.launches.CancelTaskPrompt(ctx, orgID, workspaceID, conversationID, runID, commandID)
}

func (s *DirectChatService) RespondPermission(ctx context.Context, orgID, workspaceID, conversationID, runID string, response orchestrationmodel.PermissionResponse) error {
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

func (s *DirectChatService) RespondQuestion(ctx context.Context, orgID, workspaceID, conversationID, runID string, response orchestrationmodel.QuestionResponse) error {
	if _, err := s.Get(ctx, orgID, workspaceID, conversationID); err != nil {
		return err
	}
	return s.launches.RespondTaskQuestion(ctx, orgID, workspaceID, conversationID, runID, response)
}
