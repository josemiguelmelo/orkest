package workspaceenv

import (
	"context"

	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
	workspaceenvmodel "github.com/josemiguelmelo/orkest/internal/server/workspaceenv/model"
)

type WorkspaceEnvironmentRepository interface {
	List(context.Context, string) ([]workspaceenvmodel.StoredValue, error)
	Put(context.Context, string, string, string, bool) error
	Delete(context.Context, string, string) error
	Resolve(context.Context, string) ([]workspaceenvmodel.StoredValue, error)
}

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error)
}
