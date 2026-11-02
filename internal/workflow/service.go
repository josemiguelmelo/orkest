package workflow

import (
	"context"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/core/ids"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/workspace"
)

type WorkflowDefinitionRepository interface {
	Definitions(context.Context, string) ([]Definition, error)
	Definition(context.Context, string, string) (Definition, error)
	Create(context.Context, string, Definition, func(ConfigurationSnapshot, Definition) (Definition, error)) (Definition, error)
	Save(context.Context, string, Definition, func(ConfigurationSnapshot, Definition) (Definition, error)) (Definition, error)
	Delete(context.Context, string, string, func(WorkflowDeletionSnapshot, string) error) error
}
type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error)
}
type WorkflowConfigurationService struct {
	repository WorkflowDefinitionRepository
	access     WorkspaceAccess
}

func NewWorkflowConfigurationService(repository WorkflowDefinitionRepository, access WorkspaceAccess) *WorkflowConfigurationService {
	return &WorkflowConfigurationService{repository: repository, access: access}
}

func (s *WorkflowConfigurationService) List(ctx context.Context, userID, orgID, workspaceID string) ([]Definition, error) {
	if _, err := s.access.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repository.Definitions(ctx, workspaceID)
}

func (s *WorkflowConfigurationService) Get(ctx context.Context, userID, orgID, workspaceID, workflowID string) (Definition, error) {
	if _, err := s.access.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return Definition{}, err
	}
	return s.repository.Definition(ctx, workspaceID, workflowID)
}

func (s *WorkflowConfigurationService) Create(ctx context.Context, userID, orgID, workspaceID, name string) (Definition, error) {
	w, err := s.access.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return Definition{}, err
	}
	if w.Role != "owner" && w.Role != "admin" {
		return Definition{}, modelerrors.ErrForbidden
	}
	d := DefaultDefinition(strings.TrimSpace(name))
	id, err := ids.NewUUID()
	if err != nil {
		return Definition{}, err
	}
	d.ID = id
	d.ConfiguredBy = userID
	if err := d.Validate(); err != nil {
		return Definition{}, err
	}
	return s.repository.Create(ctx, workspaceID, d, PlanConfigurationChange)
}

func (s *WorkflowConfigurationService) Save(ctx context.Context, userID, orgID, workspaceID, workflowID string, d Definition) (Definition, error) {
	w, err := s.access.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return Definition{}, err
	}
	if w.Role != "owner" && w.Role != "admin" {
		return Definition{}, modelerrors.ErrForbidden
	}
	d.ConfiguredBy = userID
	d.ID = workflowID
	return s.repository.Save(ctx, workspaceID, d, PlanConfigurationChange)
}

func (s *WorkflowConfigurationService) Delete(ctx context.Context, userID, orgID, workspaceID, workflowID string) error {
	w, err := s.access.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return err
	}
	if w.Role != "owner" && w.Role != "admin" {
		return modelerrors.ErrForbidden
	}
	return s.repository.Delete(ctx, workspaceID, workflowID, ValidateWorkflowDeletion)
}
