package workspace

import "context"

type Repository interface {
	Workspaces(context.Context, string, string) ([]Workspace, error)
	Workspace(context.Context, string, string, string) (Workspace, error)
	CreateWorkspace(context.Context, string, string, string) (Workspace, error)
	UpdateWorkspace(context.Context, string, string, string) (Workspace, error)
	DeleteWorkspace(context.Context, string, string) error
	WorkspaceMembers(context.Context, string, string) ([]Membership, error)
	SetWorkspaceMember(context.Context, string, string, string, string) error
	RemoveWorkspaceMember(context.Context, string, string, string) error
}

type OrganizationAccess interface {
	RoleForUser(context.Context, string, string) (string, error)
}
