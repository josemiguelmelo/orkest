package workspaceenv

import (
	"context"

	"github.com/josemiguelmelo/orkest/internal/workspace"
)

type Repository interface {
	List(context.Context, string) ([]StoredValue, error)
	Put(context.Context, string, string, string, bool) error
	Delete(context.Context, string, string) error
	Resolve(context.Context, string) ([]StoredValue, error)
}

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error)
}
