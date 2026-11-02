package workflow

import (
	"context"
	"errors"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/workspace"
)

type accessStub struct {
	role string
	err  error
}

func (a accessStub) GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error) {
	return workspace.Workspace{Role: a.role}, a.err
}

type repositoryStub struct {
	saved bool
	actor string
}

func (r *repositoryStub) Definitions(context.Context, string) ([]Definition, error) {
	return nil, nil
}

func (r *repositoryStub) Definition(_ context.Context, _, workflowID string) (Definition, error) {
	return Definition{ID: workflowID, Name: "Workflow", Version: 1}, nil
}

func (r *repositoryStub) Create(_ context.Context, _ string, d Definition) (Definition, error) {
	return d, nil
}

func (r *repositoryStub) Save(_ context.Context, _ string, d Definition) (Definition, error) {
	r.saved = true
	r.actor = d.ConfiguredBy
	return d, nil
}

func (r *repositoryStub) Delete(context.Context, string, string) error { return nil }

func TestConfigurationRequiresWorkspaceAdministration(t *testing.T) {
	for _, role := range []string{"viewer", "member", "admin", "owner"} {
		t.Run(role, func(t *testing.T) {
			repo := &repositoryStub{}
			s := NewService(repo, accessStub{role: role})
			d := Definition{Version: 1, ConfiguredBy: "forged", Stages: []Stage{{ID: "only", Name: "Only"}}}
			_, err := s.Save(context.Background(), "actor", "org", "workspace", "flow", d)
			allowed := role == "admin" || role == "owner"
			if allowed {
				if err != nil || !repo.saved || repo.actor != "actor" {
					t.Fatal("admin save lost actor", err)
				}
			} else if !errors.Is(err, modelerrors.ErrForbidden) || repo.saved {
				t.Fatal("non-admin saved workflow", err)
			}
		})
	}
}

func TestReadRespectsWorkspaceBoundary(t *testing.T) {
	s := NewService(&repositoryStub{}, accessStub{err: modelerrors.ErrNotFound})
	if _, err := s.Get(context.Background(), "user", "other", "workspace", "flow"); !errors.Is(err, modelerrors.ErrNotFound) {
		t.Fatal("workspace boundary ignored", err)
	}
}
