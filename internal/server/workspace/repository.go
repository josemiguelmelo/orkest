package workspace

import (
	"context"

	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type WorkspaceRepository interface {
	Workspaces(context.Context, string, string) ([]workspacemodel.Workspace, error)
	Workspace(context.Context, string, string, string) (workspacemodel.Workspace, error)
	CreateWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error)
	UpdateWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error)
	DeleteWorkspace(context.Context, string, string) error
	WorkspaceMembers(context.Context, string, string) ([]workspacemodel.Membership, error)
	SetWorkspaceMember(context.Context, string, string, string, string) error
	RemoveWorkspaceMember(context.Context, string, string, string) error
}

type OrganizationAccess interface {
	RoleForUser(context.Context, string, string) (string, error)
}
