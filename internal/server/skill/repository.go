package skill

import (
	"context"

	"github.com/josemiguelmelo/orkest/internal/server/workspace"
)

type CatalogRepository interface {
	Skills(context.Context, string, string) ([]Skill, error)
	Skill(context.Context, string, string, string) (Skill, error)
	CreateSkill(context.Context, string, Scope, string, string, string) (Skill, error)
	UpdateSkill(context.Context, string, string) (Skill, error)
	DeleteSkill(context.Context, string) error
	VisibleSkills(context.Context, string, string, []string) ([]Skill, error)
}

type OrganizationAccess interface {
	RoleForUser(context.Context, string, string) (string, error)
}

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error)
}

type PlatformAdminAccess interface {
	IsPlatformAdmin(context.Context, string) (bool, error)
}
