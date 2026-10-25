package workspace

import (
	"context"
	"errors"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
)

type workspaceRepositoryStub struct {
	roles          map[string]string
	setMemberCalls int
}

func (r *workspaceRepositoryStub) Workspaces(context.Context, string, string) ([]Workspace, error) {
	return nil, nil
}

func (r *workspaceRepositoryStub) Workspace(_ context.Context, organizationID, workspaceID, userID string) (Workspace, error) {
	role, ok := r.roles[workspaceID+":"+userID]
	if !ok {
		return Workspace{}, modelerrors.ErrNotFound
	}
	return Workspace{ID: workspaceID, OrganizationID: organizationID, Role: role}, nil
}

func (r *workspaceRepositoryStub) CreateWorkspace(context.Context, string, string, string) (Workspace, error) {
	return Workspace{}, nil
}

func (r *workspaceRepositoryStub) UpdateWorkspace(context.Context, string, string, string) (Workspace, error) {
	return Workspace{}, nil
}

func (r *workspaceRepositoryStub) DeleteWorkspace(context.Context, string, string) error { return nil }

func (r *workspaceRepositoryStub) WorkspaceMembers(context.Context, string, string) ([]Membership, error) {
	return nil, nil
}

func (r *workspaceRepositoryStub) SetWorkspaceMember(context.Context, string, string, string, string) error {
	r.setMemberCalls++
	return nil
}

func (r *workspaceRepositoryStub) RemoveWorkspaceMember(context.Context, string, string, string) error {
	return nil
}

type organizationAccessStub struct{ roles map[string]string }

func (a organizationAccessStub) RoleForUser(_ context.Context, userID, organizationID string) (string, error) {
	role, ok := a.roles[organizationID+":"+userID]
	if !ok {
		return "", modelerrors.ErrNotFound
	}
	return role, nil
}

func TestViewerCannotChangeWorkspaceMembership(t *testing.T) {
	repository := &workspaceRepositoryStub{roles: map[string]string{"workspace:viewer": "viewer"}}
	service := NewWorkspaceService(repository, organizationAccessStub{roles: map[string]string{}})
	err := service.SetWorkspaceMember(context.Background(), "viewer", "org", "workspace", "other", "member")
	if !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
	if repository.setMemberCalls != 0 {
		t.Fatal("forbidden update reached repository")
	}
}

func TestMemberCannotManageWorkspaceMembership(t *testing.T) {
	repository := &workspaceRepositoryStub{roles: map[string]string{"workspace:member": "member"}}
	service := NewWorkspaceService(repository, organizationAccessStub{roles: map[string]string{}})
	err := service.SetWorkspaceMember(context.Background(), "member", "org", "workspace", "other", "viewer")
	if !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}
