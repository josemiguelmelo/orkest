package workflow

import (
	"errors"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
)

func TestTransitionRules(t *testing.T) {
	d := Definition{ID: "test", Name: "Test", Version: 1, Stages: []Stage{
		{ID: "ready", Name: "Ready"},
		{ID: "build", Name: "Build", ResponsibleAgentID: "builder", AutoStart: true, AutoAdvance: true},
		{ID: "verify", Name: "Verify", ResponsibleAgentID: "reviewer", AutoStart: true},
		{ID: "closed", Name: "Closed"},
	}}
	tests := []struct {
		name, current, target, assignee string
		trigger                         Trigger
		want                            Plan
		conflict                        bool
	}{
		{name: "entry assigns and starts", current: "ready", target: "build", trigger: Enter, want: Plan{StageID: "build", AssigneeAgentID: "builder", Start: true}},
		{name: "success enters next stage", current: "build", assignee: "builder", trigger: Succeeded, want: Plan{StageID: "verify", AssigneeAgentID: "reviewer", Start: true}},
		{name: "success stays when disabled", current: "verify", assignee: "reviewer", trigger: Succeeded, want: Plan{StageID: "verify", AssigneeAgentID: "reviewer", Completed: true}},
		{name: "manual completion stays by default", current: "ready", trigger: Complete, want: Plan{StageID: "ready", Completed: true}},
		{name: "unconfigured assignment is retained", current: "verify", target: "ready", assignee: "reviewer", trigger: Enter, want: Plan{StageID: "ready", AssigneeAgentID: "reviewer"}},
		{name: "start needs assignee", current: "ready", trigger: Start, conflict: true},
		{name: "closed task cannot reopen", current: "closed", target: "ready", trigger: Enter, conflict: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PlanTransition(d, tt.current, tt.target, tt.assignee, tt.trigger)
			if tt.conflict {
				if !errors.Is(err, modelerrors.ErrConflict) {
					t.Fatal(err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

func TestDefinitionValidation(t *testing.T) {
	d := Definition{ID: "test", Name: "Test", Version: 1, Stages: []Stage{{ID: "only", Name: "Only"}}}
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	d.Stages[0].AutoStart = true
	if !errors.Is(d.Validate(), modelerrors.ErrInvalid) {
		t.Fatal("terminal automation accepted")
	}
	d.Stages[0].AutoStart = false
	d.Stages = append(d.Stages, d.Stages[0])
	if !errors.Is(d.Validate(), modelerrors.ErrInvalid) {
		t.Fatal("duplicate stage IDs accepted")
	}
}

func TestWorkflowEndpointsFollowStageOrder(t *testing.T) {
	d := Definition{Stages: []Stage{{ID: "intake"}, {ID: "active"}, {ID: "complete"}}}
	if d.StartStageID() != "intake" || d.FinalStageID() != "complete" {
		t.Fatalf("unexpected workflow endpoints: start=%q final=%q", d.StartStageID(), d.FinalStageID())
	}
	if _, err := PlanTransition(d, "complete", "intake", "agent", Enter); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatalf("final stage reopened: %v", err)
	}
	d.Stages = []Stage{{ID: "intake"}, {ID: "complete", AutoAdvance: true}}
	if !errors.Is(d.Validate(), modelerrors.ErrInvalid) {
		t.Fatal("final stage automation accepted")
	}
}
