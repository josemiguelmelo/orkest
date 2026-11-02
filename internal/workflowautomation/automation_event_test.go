package workflowautomation

import (
	"context"
	"errors"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/task"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	definition "github.com/josemiguelmelo/orkest/internal/workflow"
)

type workflowSnapshotRepository struct {
	AutomationEventRepository
	snapshot     AutomationEventSnapshot
	decision     AutomationEventEffects
	acknowledged bool
}

func (r *workflowSnapshotRepository) WithAutomationEvent(_ context.Context, decide func(AutomationEventSnapshot) (AutomationEventEffects, error)) (bool, error) {
	decision, err := decide(r.snapshot)
	if err != nil {
		return false, err
	}
	r.decision = decision
	r.acknowledged = true
	return true, nil
}

type eventEvaluatorStub struct {
	calls int
	plan  StageTransitionPlan
	err   error
}

func (e *eventEvaluatorStub) Evaluate(definition.Definition, StageTransitionPlan, Event) (StageTransitionPlan, error) {
	e.calls++
	return e.plan, e.err
}

func TestServiceDecidesWorkflowEvents(t *testing.T) {
	for _, test := range []struct {
		name      string
		change    func(*AutomationEventSnapshot)
		evaluated bool
		state     task.WorkState
		apply     bool
	}{
		{"entry", func(s *AutomationEventSnapshot) {
			s.Event.Type = definition.StageEntered
			s.RunID = ""
			s.WorkState = task.WorkStateIdle
		}, true, "", true},
		{"success", func(*AutomationEventSnapshot) {}, true, "", true},
		{"stale stage", func(s *AutomationEventSnapshot) { s.Event.StageVersion-- }, false, "", false},
		{"old run", func(s *AutomationEventSnapshot) { s.Event.RunID = "old" }, false, "", false},
		{"already completed", func(s *AutomationEventSnapshot) { s.WorkState = task.WorkStateCompleted }, false, "", false},
		{"failure", func(s *AutomationEventSnapshot) { s.Event.Type = definition.RunFailed }, false, task.WorkStateFailed, false},
		{"entry superseded", func(s *AutomationEventSnapshot) {
			s.Event.Type = definition.StageEntered
			s.WorkState = task.WorkStateQueued
		}, false, "", false},
		{"manual completion superseded", func(s *AutomationEventSnapshot) {
			s.Event.Type = definition.StageCompleted
			s.WorkState = task.WorkStateCompleted
			s.Event.RunID = "old"
		}, false, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := AutomationEventSnapshot{Workflow: definition.Definition{ConfiguredBy: "admin"}, StageID: "in_progress", StageVersion: 2, RunID: "run", WorkState: task.WorkStateRunning, Event: AutomationEvent{Type: definition.RunSucceeded, StageVersion: 2, RunID: "run"}}
			test.change(&snapshot)
			repo := &workflowSnapshotRepository{snapshot: snapshot}
			evaluator := &eventEvaluatorStub{plan: StageTransitionPlan{StageID: "in_review", RuleID: "rule"}}
			service := NewWorkflowAutomationService(nil, repo, nil, evaluator)
			found, err := service.ProcessNextEvent(context.Background())
			if err != nil || !found || !repo.acknowledged {
				t.Fatal(found, err)
			}
			if (evaluator.calls > 0) != test.evaluated || (repo.decision.Plan != nil) != test.apply || repo.decision.WorkState != test.state {
				t.Fatalf("unexpected service decision: %+v, calls=%d", repo.decision, evaluator.calls)
			}
			if test.apply && (repo.decision.ActorID != "admin" || repo.decision.RuleID != "rule") {
				t.Fatal("service lost automation identity", repo.decision)
			}
		})
	}
}

func TestServiceHandlesEvaluationFailuresAndActiveLaunch(t *testing.T) {
	for _, test := range []struct {
		name    string
		err     error
		active  bool
		wantErr bool
		state   task.WorkState
	}{
		{name: "business conflict", err: modelerrors.ErrConflict, state: task.WorkStateFailed},
		{name: "evaluation failure", err: errors.New("evaluator unavailable"), wantErr: true},
		{name: "active launch", active: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &workflowSnapshotRepository{snapshot: AutomationEventSnapshot{Workflow: definition.Definition{ConfiguredBy: "admin"}, StageID: "in_progress", StageVersion: 1, WorkState: task.WorkStateIdle, ActiveLaunch: test.active, Event: AutomationEvent{Type: definition.StageEntered, StageVersion: 1, ActorID: "actor"}}}
			evaluator := &eventEvaluatorStub{plan: StageTransitionPlan{StageID: "in_progress", Start: true, RuleID: "rule"}, err: test.err}
			service := NewWorkflowAutomationService(nil, repo, nil, evaluator)
			_, err := service.ProcessNextEvent(context.Background())
			if (err != nil) != test.wantErr || repo.acknowledged == test.wantErr {
				t.Fatal("incorrect acknowledgement", err)
			}
			if !test.wantErr && (repo.decision.Plan != nil || repo.decision.WorkState != test.state || repo.decision.RuleID != "rule") {
				t.Fatal("incorrect failure/active policy", repo.decision)
			}
		})
	}
}
