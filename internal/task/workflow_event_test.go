package task

import (
	"context"
	"errors"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/workflow"
)

type workflowSnapshotRepository struct {
	TransitionRepository
	snapshot     WorkflowEventSnapshot
	decision     WorkflowEventDecision
	acknowledged bool
}

func (r *workflowSnapshotRepository) WithWorkflowEvent(_ context.Context, decide func(WorkflowEventSnapshot) (WorkflowEventDecision, error)) (bool, error) {
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
	plan  workflow.Plan
	err   error
}

func (e *eventEvaluatorStub) Evaluate(workflow.Definition, workflow.Plan, workflow.Event) (workflow.Plan, error) {
	e.calls++
	return e.plan, e.err
}

func TestServiceDecidesWorkflowEvents(t *testing.T) {
	for _, test := range []struct {
		name      string
		change    func(*WorkflowEventSnapshot)
		evaluated bool
		state     WorkState
		apply     bool
	}{
		{"entry", func(s *WorkflowEventSnapshot) {
			s.Event.Type = workflow.StageEntered
			s.RunID = ""
			s.WorkState = WorkStateIdle
		}, true, "", true},
		{"success", func(*WorkflowEventSnapshot) {}, true, "", true},
		{"stale stage", func(s *WorkflowEventSnapshot) { s.Event.StageVersion-- }, false, "", false},
		{"old run", func(s *WorkflowEventSnapshot) { s.Event.RunID = "old" }, false, "", false},
		{"already completed", func(s *WorkflowEventSnapshot) { s.WorkState = WorkStateCompleted }, false, "", false},
		{"failure", func(s *WorkflowEventSnapshot) { s.Event.Type = workflow.RunFailed }, false, WorkStateFailed, false},
		{"entry superseded", func(s *WorkflowEventSnapshot) { s.Event.Type = workflow.StageEntered; s.WorkState = WorkStateQueued }, false, "", false},
		{"manual completion superseded", func(s *WorkflowEventSnapshot) {
			s.Event.Type = workflow.StageCompleted
			s.WorkState = WorkStateCompleted
			s.Event.RunID = "old"
		}, false, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := WorkflowEventSnapshot{Workflow: workflow.Definition{ConfiguredBy: "admin"}, StageID: "in_progress", StageVersion: 2, RunID: "run", WorkState: WorkStateRunning, Event: WorkflowEvent{Type: workflow.RunSucceeded, StageVersion: 2, RunID: "run"}}
			test.change(&snapshot)
			repo := &workflowSnapshotRepository{snapshot: snapshot}
			evaluator := &eventEvaluatorStub{plan: workflow.Plan{StageID: "in_review", RuleID: "rule"}}
			service := NewService(nil, nil, nil, repo, evaluator)
			found, err := service.ProcessWorkflowEvent(context.Background())
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
		state   WorkState
	}{
		{name: "business conflict", err: modelerrors.ErrConflict, state: WorkStateFailed},
		{name: "evaluation failure", err: errors.New("evaluator unavailable"), wantErr: true},
		{name: "active launch", active: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &workflowSnapshotRepository{snapshot: WorkflowEventSnapshot{Workflow: workflow.Definition{ConfiguredBy: "admin"}, StageID: "in_progress", StageVersion: 1, WorkState: WorkStateIdle, ActiveLaunch: test.active, Event: WorkflowEvent{Type: workflow.StageEntered, StageVersion: 1, ActorID: "actor"}}}
			evaluator := &eventEvaluatorStub{plan: workflow.Plan{StageID: "in_progress", Start: true, RuleID: "rule"}, err: test.err}
			service := NewService(nil, nil, nil, repo, evaluator)
			_, err := service.ProcessWorkflowEvent(context.Background())
			if (err != nil) != test.wantErr || repo.acknowledged == test.wantErr {
				t.Fatal("incorrect acknowledgement", err)
			}
			if !test.wantErr && (repo.decision.Plan != nil || repo.decision.WorkState != test.state || repo.decision.RuleID != "rule") {
				t.Fatal("incorrect failure/active policy", repo.decision)
			}
		})
	}
}
