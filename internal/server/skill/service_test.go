package skill

import (
	"context"
	"errors"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	skillmodel "github.com/josemiguelmelo/orkest/internal/server/skill/model"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type repositoryStub struct {
	created skillmodel.Scope
	skillMD string
}

func (r *repositoryStub) Skills(context.Context, string, string) ([]skillmodel.Skill, error) {
	return nil, nil
}

func (r *repositoryStub) Skill(context.Context, string, string, string) (skillmodel.Skill, error) {
	return skillmodel.Skill{}, modelerrors.ErrNotFound
}

func (r *repositoryStub) CreateSkill(_ context.Context, _ string, scope skillmodel.Scope, _, _, skillMD string) (skillmodel.Skill, error) {
	r.created, r.skillMD = scope, skillMD
	return skillmodel.Skill{}, nil
}

func (r *repositoryStub) UpdateSkill(context.Context, string, string) (skillmodel.Skill, error) {
	return skillmodel.Skill{}, nil
}

func (r *repositoryStub) DeleteSkill(context.Context, string) error { return nil }

func (r *repositoryStub) VisibleSkills(context.Context, string, string, []string) ([]skillmodel.Skill, error) {
	return nil, nil
}

type organizationStub struct {
	role string
	err  error
}

func (s organizationStub) RoleForUser(context.Context, string, string) (string, error) {
	return s.role, s.err
}

type workspaceStub struct {
	role string
	err  error
}

func (s workspaceStub) GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error) {
	return workspacemodel.Workspace{Role: s.role}, s.err
}

type platformAdminStub bool

func (s platformAdminStub) IsPlatformAdmin(context.Context, string) (bool, error) {
	return bool(s), nil
}

const validSkill = "---\nname: go-review\ndescription: Review Go code for correctness and clarity\n---\n\nReview the change and report issues."

func TestValidateContent(t *testing.T) {
	id, err := validateContent(validSkill)
	if err != nil || id != "go-review" {
		t.Fatalf("validateContent() = %q, %v", id, err)
	}
	for _, content := range []string{"", "---\nname: Bad Name\ndescription: description\n---\nbody", "---\nname: no-body\ndescription: description\n---\n"} {
		if _, err = validateContent(content); !errors.Is(err, modelerrors.ErrInvalid) {
			t.Errorf("validateContent(%q) error = %v", content, err)
		}
	}
}

func TestCreateRequiresScopeAdministrator(t *testing.T) {
	repository := &repositoryStub{}
	service := NewCatalogService(repository, organizationStub{role: "admin"}, workspaceStub{role: "admin"}, platformAdminStub(false))
	if _, err := service.Create(context.Background(), "user", skillmodel.ScopeOrganization, "org", "", validSkill); err != nil {
		t.Fatalf("organization admin create: %v", err)
	}
	if repository.created != skillmodel.ScopeOrganization || repository.skillMD != validSkill {
		t.Fatalf("unexpected create: %#v", repository)
	}

	service = NewCatalogService(repository, organizationStub{role: "member"}, workspaceStub{role: "member"}, platformAdminStub(false))
	if _, err := service.Create(context.Background(), "user", skillmodel.ScopeOrganization, "org", "", validSkill); !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatalf("organization member create error = %v", err)
	}
	if _, err := service.Create(context.Background(), "user", skillmodel.ScopeGlobal, "", "", validSkill); !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatalf("non-platform-admin global create error = %v", err)
	}
}

func TestCreateGlobalRequiresPlatformAdmin(t *testing.T) {
	service := NewCatalogService(&repositoryStub{}, organizationStub{}, workspaceStub{}, platformAdminStub(true))
	if _, err := service.Create(context.Background(), "user", skillmodel.ScopeGlobal, "", "", validSkill); err != nil {
		t.Fatalf("platform admin create: %v", err)
	}
}
