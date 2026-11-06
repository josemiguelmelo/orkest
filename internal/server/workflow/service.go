package workflow

import (
	"context"
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type WorkflowDefinitionRepository interface {
	Definitions(context.Context, string) ([]workflowmodel.Definition, error)
	Definition(context.Context, string, string) (workflowmodel.Definition, error)
	Create(context.Context, string, workflowmodel.Definition, func(ConfigurationSnapshot, workflowmodel.Definition) (workflowmodel.Definition, error)) (workflowmodel.Definition, error)
	Save(context.Context, string, workflowmodel.Definition, func(ConfigurationSnapshot, workflowmodel.Definition) (workflowmodel.Definition, error)) (workflowmodel.Definition, error)
	Delete(context.Context, string, string, func(WorkflowDeletionSnapshot, string) error) error
}

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error)
}

type WorkflowConfigurationService struct {
	repository WorkflowDefinitionRepository
	access     WorkspaceAccess
}

func NewWorkflowConfigurationService(repository WorkflowDefinitionRepository, access WorkspaceAccess) *WorkflowConfigurationService {
	return &WorkflowConfigurationService{repository: repository, access: access}
}

func (s *WorkflowConfigurationService) List(ctx context.Context, userID, orgID, workspaceID string) ([]workflowmodel.Definition, error) {
	if _, err := s.access.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repository.Definitions(ctx, workspaceID)
}

func (s *WorkflowConfigurationService) Get(ctx context.Context, userID, orgID, workspaceID, workflowID string) (workflowmodel.Definition, error) {
	if _, err := s.access.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return workflowmodel.Definition{}, err
	}
	return s.repository.Definition(ctx, workspaceID, workflowID)
}

func (s *WorkflowConfigurationService) Create(ctx context.Context, userID, orgID, workspaceID, name string) (workflowmodel.Definition, error) {
	w, err := s.access.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return workflowmodel.Definition{}, err
	}
	if w.Role != "owner" && w.Role != "admin" {
		return workflowmodel.Definition{}, modelerrors.ErrForbidden
	}
	d := workflowmodel.DefaultDefinition(strings.TrimSpace(name))
	id, err := ids.NewUUID()
	if err != nil {
		return workflowmodel.Definition{}, err
	}
	d.ID = id
	d.ConfiguredBy = userID
	if err := d.Validate(); err != nil {
		return workflowmodel.Definition{}, err
	}
	return s.repository.Create(ctx, workspaceID, d, PlanConfigurationChange)
}

func (s *WorkflowConfigurationService) Save(ctx context.Context, userID, orgID, workspaceID, workflowID string, d workflowmodel.Definition) (workflowmodel.Definition, error) {
	w, err := s.access.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return workflowmodel.Definition{}, err
	}
	if w.Role != "owner" && w.Role != "admin" {
		return workflowmodel.Definition{}, modelerrors.ErrForbidden
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
