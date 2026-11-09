package workspacemount

import (
	"context"
	"fmt"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
	mountmodel "github.com/josemiguelmelo/orkest/internal/server/workspacemount/model"
)

type WorkspaceMountRepository interface {
	List(context.Context, string) ([]mountmodel.Mount, error)
	Put(context.Context, string, []mountmodel.Mount) error
}

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error)
}

type WorkspaceMountService struct {
	repository WorkspaceMountRepository
	workspaces WorkspaceAccess
	enabled    bool
}

func New(repository WorkspaceMountRepository, workspaces WorkspaceAccess, enabled bool) *WorkspaceMountService {
	return &WorkspaceMountService{repository: repository, workspaces: workspaces, enabled: enabled}
}

func (s *WorkspaceMountService) Get(ctx context.Context, userID, orgID, workspaceID string) (mountmodel.Settings, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return mountmodel.Settings{}, err
	}
	mounts, err := s.repository.List(ctx, workspaceID)
	return mountmodel.Settings{Enabled: s.enabled, Mounts: mounts}, err
}

func (s *WorkspaceMountService) Put(ctx context.Context, userID, orgID, workspaceID string, mounts []mountmodel.Mount) error {
	w, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return err
	}
	if w.Role != "admin" && w.Role != "owner" {
		return modelerrors.ErrForbidden
	}
	if err := s.validate(mounts); err != nil {
		return err
	}
	return s.repository.Put(ctx, workspaceID, mounts)
}

func (s *WorkspaceMountService) Resolve(ctx context.Context, workspaceID string) ([]mountmodel.Mount, error) {
	mounts, err := s.repository.List(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if err := s.validate(mounts); err != nil {
		return nil, err
	}
	return mounts, nil
}

func (s *WorkspaceMountService) validate(mounts []mountmodel.Mount) error {
	if err := mountmodel.Validate(mounts); err != nil {
		return fmt.Errorf("%w: %s", modelerrors.ErrInvalid, err)
	}
	if len(mounts) > 0 && !s.enabled {
		return fmt.Errorf("%w: host mounts require the Docker executor", modelerrors.ErrInvalid)
	}
	return nil
}
