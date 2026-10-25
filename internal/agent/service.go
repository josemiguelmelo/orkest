package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/core/ids"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
)

type Service struct {
	repository Repository
	workspaces WorkspaceAccess
}

func NewService(repository Repository, workspaces WorkspaceAccess) *Service {
	return &Service{repository: repository, workspaces: workspaces}
}

func (s *Service) List(ctx context.Context, userID, orgID, workspaceID string) ([]Agent, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repository.Agents(ctx, orgID, workspaceID)
}

func (s *Service) Get(ctx context.Context, userID, orgID, workspaceID, agentID string) (Agent, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return Agent{}, err
	}
	return s.repository.Agent(ctx, orgID, workspaceID, agentID)
}

func (s *Service) Create(ctx context.Context, userID, orgID, workspaceID string, definition AgentDefinition) (Agent, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return Agent{}, err
	}
	if err := validate(&definition); err != nil {
		return Agent{}, err
	}
	id, err := ids.NewUUID()
	if err != nil {
		return Agent{}, fmt.Errorf("generate agent ID: %w", err)
	}
	return s.repository.CreateAgent(ctx, id, workspaceID, definition)
}

func (s *Service) Update(ctx context.Context, userID, orgID, workspaceID, agentID string, definition AgentDefinition) (Agent, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return Agent{}, err
	}
	if err := validate(&definition); err != nil {
		return Agent{}, err
	}
	return s.repository.UpdateAgent(ctx, orgID, workspaceID, agentID, definition)
}

func (s *Service) Delete(ctx context.Context, userID, orgID, workspaceID, agentID string) error {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return err
	}
	return s.repository.DeleteAgent(ctx, orgID, workspaceID, agentID)
}

func (s *Service) authorize(ctx context.Context, userID, orgID, workspaceID string) error {
	w, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return err
	}
	if w.Role == "viewer" {
		return modelerrors.ErrForbidden
	}
	return nil
}

func validate(definition *AgentDefinition) error {
	definition.Name = strings.TrimSpace(definition.Name)
	definition.Provider = strings.TrimSpace(definition.Provider)
	definition.Model = strings.TrimSpace(definition.Model)
	definition.Description = strings.TrimSpace(definition.Description)
	if definition.Name == "" || len(definition.Name) > 200 || definition.Provider == "" || len(definition.Provider) > 200 || definition.Model == "" || len(definition.Model) > 200 || len(definition.Description) > 2000 || len(definition.SystemPrompt) > 100000 || !definition.Runtime.Available() {
		return modelerrors.ErrInvalid
	}
	return nil
}
