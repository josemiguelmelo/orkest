package skill

import (
	"context"

	skillmodel "github.com/josemiguelmelo/orkest/internal/server/skill/model"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type CatalogRepository interface {
	Skills(context.Context, string, string) ([]skillmodel.Skill, error)
	Skill(context.Context, string, string, string) (skillmodel.Skill, error)
	CreateSkill(context.Context, string, skillmodel.Scope, string, string, string) (skillmodel.Skill, error)
	UpdateSkill(context.Context, string, string) (skillmodel.Skill, error)
	DeleteSkill(context.Context, string) error
	VisibleSkills(context.Context, string, string, []string) ([]skillmodel.Skill, error)
}

type OrganizationAccess interface {
	RoleForUser(context.Context, string, string) (string, error)
}

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error)
}

type PlatformAdminAccess interface {
	IsPlatformAdmin(context.Context, string) (bool, error)
}
