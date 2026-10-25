package workspace

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/core/ids"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
)

type WorkspaceService struct {
	repository    Repository
	organizations OrganizationAccess
}

func NewWorkspaceService(repository Repository, organizations OrganizationAccess) *WorkspaceService {
	return &WorkspaceService{repository: repository, organizations: organizations}
}

func normalizeName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	return name, name != "" && len(name) <= 200
}

func (s *WorkspaceService) ListWorkspaces(ctx context.Context, userID, orgID string) ([]Workspace, error) {
	return s.repository.Workspaces(ctx, orgID, userID)
}

func (s *WorkspaceService) GetWorkspace(ctx context.Context, userID, orgID, workspaceID string) (Workspace, error) {
	return s.repository.Workspace(ctx, orgID, workspaceID, userID)
}

func (s *WorkspaceService) CreateWorkspace(ctx context.Context, userID, orgID, name string) (Workspace, error) {
	var valid bool
	if name, valid = normalizeName(name); !valid {
		return Workspace{}, modelerrors.ErrInvalid
	}
	role, err := s.organizations.RoleForUser(ctx, userID, orgID)
	if err != nil {
		return Workspace{}, err
	}
	if role != "owner" && role != "admin" {
		return Workspace{}, modelerrors.ErrForbidden
	}
	id, err := ids.NewUUID()
	if err != nil {
		return Workspace{}, fmt.Errorf("generate workspace ID: %w", err)
	}
	return s.repository.CreateWorkspace(ctx, id, orgID, name)
}

func (s *WorkspaceService) UpdateWorkspace(ctx context.Context, userID, orgID, workspaceID, name string) (Workspace, error) {
	var valid bool
	if name, valid = normalizeName(name); !valid {
		return Workspace{}, modelerrors.ErrInvalid
	}
	if err := s.requireWorkspaceAdmin(ctx, userID, orgID, workspaceID); err != nil {
		return Workspace{}, err
	}
	return s.repository.UpdateWorkspace(ctx, orgID, workspaceID, name)
}

func (s *WorkspaceService) DeleteWorkspace(ctx context.Context, userID, orgID, workspaceID string) error {
	if err := s.requireWorkspaceAdmin(ctx, userID, orgID, workspaceID); err != nil {
		return err
	}
	return s.repository.DeleteWorkspace(ctx, orgID, workspaceID)
}

func (s *WorkspaceService) ListWorkspaceMembers(ctx context.Context, userID, orgID, workspaceID string) ([]Membership, error) {
	if _, err := s.repository.Workspace(ctx, orgID, workspaceID, userID); err != nil {
		return nil, err
	}
	return s.repository.WorkspaceMembers(ctx, orgID, workspaceID)
}

func (s *WorkspaceService) SetWorkspaceMember(ctx context.Context, actorID, orgID, workspaceID, targetID, role string) error {
	if role != "admin" && role != "member" && role != "viewer" {
		return modelerrors.ErrInvalid
	}
	if err := s.requireWorkspaceAdmin(ctx, actorID, orgID, workspaceID); err != nil {
		return err
	}
	return s.repository.SetWorkspaceMember(ctx, orgID, workspaceID, targetID, role)
}

func (s *WorkspaceService) RemoveWorkspaceMember(ctx context.Context, actorID, orgID, workspaceID, targetID string) error {
	if err := s.requireWorkspaceAdmin(ctx, actorID, orgID, workspaceID); err != nil {
		return err
	}
	return s.repository.RemoveWorkspaceMember(ctx, orgID, workspaceID, targetID)
}

func (s *WorkspaceService) requireWorkspaceAdmin(ctx context.Context, userID, orgID, workspaceID string) error {
	role, err := s.organizations.RoleForUser(ctx, userID, orgID)
	if err == nil && (role == "owner" || role == "admin") {
		_, e := s.repository.Workspace(ctx, orgID, workspaceID, userID)
		return e
	}
	if err != nil && !errors.Is(err, modelerrors.ErrNotFound) {
		return err
	}
	w, err := s.repository.Workspace(ctx, orgID, workspaceID, userID)
	if err != nil {
		return err
	}
	if w.Role != "admin" {
		return modelerrors.ErrForbidden
	}
	return nil
}
