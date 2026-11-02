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

func (r *repositoryStub) Create(_ context.Context, _ string, d Definition, decide func(ConfigurationSnapshot, Definition) (Definition, error)) (Definition, error) {
	return decide(ConfigurationSnapshot{}, d)
}

func (r *repositoryStub) Save(_ context.Context, _ string, d Definition, decide func(ConfigurationSnapshot, Definition) (Definition, error)) (Definition, error) {
	var err error
	current := Definition{ID: d.ID, Name: "Workflow", Version: d.Version}
	d, err = decide(ConfigurationSnapshot{Current: &current}, d)
	if err != nil {
		return Definition{}, err
	}
	r.saved = true
	r.actor = d.ConfiguredBy
	return d, nil
}

func (r *repositoryStub) Delete(context.Context, string, string, func(WorkflowDeletionSnapshot, string) error) error {
	return nil
}

func TestConfigurationRequiresWorkspaceAdministration(t *testing.T) {
	for _, role := range []string{"viewer", "member", "admin", "owner"} {
		t.Run(role, func(t *testing.T) {
			repo := &repositoryStub{}
			s := NewWorkflowConfigurationService(repo, accessStub{role: role})
			d := Definition{InitialStageID: "only", ClosedStageID: "only", Version: 1, ConfiguredBy: "forged", Stages: []Stage{{ID: "only", Name: "Only"}}}
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
	s := NewWorkflowConfigurationService(&repositoryStub{}, accessStub{err: modelerrors.ErrNotFound})
	if _, err := s.Get(context.Background(), "user", "other", "workspace", "flow"); !errors.Is(err, modelerrors.ErrNotFound) {
		t.Fatal("workspace boundary ignored", err)
	}
}

func TestConfigurationChangeValidatesLockedReferences(t *testing.T) {
	original := DefaultDefinition("Board")
	original.ID = "flow"
	cases := []struct {
		name   string
		change func(*ConfigurationSnapshot, *Definition)
		want   error
	}{
		{"stale version", func(s *ConfigurationSnapshot, d *Definition) { d.Version++ }, modelerrors.ErrConflict},
		{"unknown agent", func(s *ConfigurationSnapshot, d *Definition) { d.Stages[0].ResponsibleAgentID = "missing" }, modelerrors.ErrInvalid},
		{"occupied stage removal", func(s *ConfigurationSnapshot, d *Definition) {
			s.OccupiedStageIDs = []string{"in_review"}
			d.Stages = append(d.Stages[:2], d.Stages[3:]...)
			d.Rules = nil
		}, modelerrors.ErrConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			current := original
			input := original
			input.Stages = append([]Stage(nil), original.Stages...)
			snapshot := ConfigurationSnapshot{Current: &current}
			tc.change(&snapshot, &input)
			if _, err := PlanConfigurationChange(snapshot, input); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestWorkflowDeletionUsesLockedWorkspaceReferences(t *testing.T) {
	for _, tc := range []struct {
		name     string
		snapshot WorkflowDeletionSnapshot
		want     error
	}{
		{"missing", WorkflowDeletionSnapshot{WorkflowIDs: []string{"other"}}, modelerrors.ErrNotFound},
		{"last workflow", WorkflowDeletionSnapshot{WorkflowIDs: []string{"flow"}}, modelerrors.ErrConflict},
		{"referenced", WorkflowDeletionSnapshot{WorkflowIDs: []string{"flow", "other"}, Referenced: true}, modelerrors.ErrConflict},
		{"unreferenced", WorkflowDeletionSnapshot{WorkflowIDs: []string{"flow", "other"}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateWorkflowDeletion(tc.snapshot, "flow"); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}
