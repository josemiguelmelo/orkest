package workflow

import (
	"errors"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
)

func TestExplicitDestinationsSurviveReorder(t *testing.T) {
	d := DefaultDefinition("Test")
	d.ID = "flow"
	d.Stages[0], d.Stages[2] = d.Stages[2], d.Stages[0]
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	p, err := PlanTransition(d, "in_progress", "", "agent", Succeeded)
	if err != nil || p.StageID != "in_review" || p.Start {
		t.Fatalf("unexpected success plan: %+v %v", p, err)
	}
	if d.StartStageID() != "todo" || d.FinalStageID() != "done" {
		t.Fatal("reorder changed endpoints")
	}
	if _, err = PlanTransition(d, "done", "todo", "agent", Enter); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatal("closed task reopened", err)
	}
}

func TestStageEntryAndExecutionAreSeparate(t *testing.T) {
	d := DefaultDefinition("Test")
	d.ID = "flow"
	d.Stages[1].ResponsibleAgentID = "builder"
	p, err := PlanTransition(d, "todo", "in_progress", "", Enter)
	if err != nil || p.AssigneeAgentID != "builder" || p.Start {
		t.Fatal(p, err)
	}
	var evaluator AutomationEvaluator = RuleEvaluator{}
	p, err = evaluator.Evaluate(d, p, Event{Type: StageEntered})
	if err != nil || !p.Start {
		t.Fatal(p, err)
	}
	if _, err = evaluator.Evaluate(d, Plan{StageID: "in_progress"}, Event{Type: StageEntered}); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatal("missing assignment accepted", err)
	}
	p, err = evaluator.Evaluate(d, Plan{StageID: "in_progress"}, Event{Type: RunFailed})
	if err != nil || p.Start || p.Completed || p.StageID != "in_progress" {
		t.Fatal("failure advanced", p, err)
	}
}

func TestRuleValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Definition)
	}{
		{"duplicate enabled match", func(d *Definition) { r := d.Rules[0]; r.ID = "other"; d.Rules = append(d.Rules, r) }},
		{"entry cannot move", func(d *Definition) { d.Rules[0].Action = MoveTask; d.Rules[0].TargetStageID = "todo" }},
		{"missing destination", func(d *Definition) { d.Rules[1].TargetStageID = "absent" }},
		{"self movement", func(d *Definition) { d.Rules[1].TargetStageID = "in_progress" }},
		{"closed automation", func(d *Definition) { d.Rules[0].SourceStageID = "done" }},
		{"missing initial", func(d *Definition) { d.InitialStageID = "absent" }},
		{"duplicate stage", func(d *Definition) { d.Stages = append(d.Stages, d.Stages[0]) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			d := DefaultDefinition("Test")
			d.ID = "flow"
			test.change(&d)
			if !errors.Is(d.Validate(), modelerrors.ErrInvalid) {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	d := DefaultDefinition("Test")
	d.ID = "flow"
	d.Rules = nil
	if err := d.Validate(); err != nil {
		t.Fatal("manual board rejected", err)
	}
}

func TestManualCompletionHasIndependentPolicy(t *testing.T) {
	d := DefaultDefinition("Test")
	d.Rules = d.Rules[:2]
	p, err := PlanTransition(d, "in_progress", "", "agent", Complete)
	if err != nil || p.StageID != "in_progress" || !p.Completed {
		t.Fatal(p, err)
	}
}
