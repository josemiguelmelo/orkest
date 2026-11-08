package workflowautomation

import (
	"context"
	"errors"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
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

func (e *eventEvaluatorStub) Evaluate(workflowmodel.Definition, StageTransitionPlan, Event) (StageTransitionPlan, error) {
	e.calls++
	return e.plan, e.err
}

func TestServiceDecidesWorkflowEvents(t *testing.T) {
	for _, test := range []struct {
		name      string
		change    func(*AutomationEventSnapshot)
		evaluated bool
		state     taskmodel.WorkState
		apply     bool
	}{
		{"entry", func(s *AutomationEventSnapshot) {
			s.Event.Type = workflowmodel.StageEntered
			s.RunID = ""
			s.WorkState = taskmodel.WorkStateIdle
		}, true, "", true},
		{"success", func(*AutomationEventSnapshot) {}, true, "", true},
		{"stale stage", func(s *AutomationEventSnapshot) { s.Event.StageVersion-- }, false, "", false},
		{"old run", func(s *AutomationEventSnapshot) { s.Event.RunID = "old" }, false, "", false},
		{"already completed", func(s *AutomationEventSnapshot) { s.WorkState = taskmodel.WorkStateCompleted }, false, "", false},
		{"failure", func(s *AutomationEventSnapshot) { s.Event.Type = workflowmodel.RunFailed }, false, taskmodel.WorkStateFailed, false},
		{"entry superseded", func(s *AutomationEventSnapshot) {
			s.Event.Type = workflowmodel.StageEntered
			s.WorkState = taskmodel.WorkStateQueued
		}, false, "", false},
		{"manual completion superseded", func(s *AutomationEventSnapshot) {
			s.Event.Type = workflowmodel.StageCompleted
			s.WorkState = taskmodel.WorkStateCompleted
			s.Event.RunID = "old"
		}, false, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := AutomationEventSnapshot{Workflow: workflowmodel.Definition{ConfiguredBy: "admin"}, StageID: "in_progress", StageVersion: 2, RunID: "run", WorkState: taskmodel.WorkStateRunning, Event: AutomationEvent{Type: workflowmodel.RunSucceeded, StageVersion: 2, RunID: "run"}}
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
		state   taskmodel.WorkState
	}{
		{name: "business conflict", err: modelerrors.ErrConflict, state: taskmodel.WorkStateFailed},
		{name: "evaluation failure", err: errors.New("evaluator unavailable"), wantErr: true},
		{name: "active launch", active: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &workflowSnapshotRepository{snapshot: AutomationEventSnapshot{Workflow: workflowmodel.Definition{ConfiguredBy: "admin"}, StageID: "in_progress", StageVersion: 1, WorkState: taskmodel.WorkStateIdle, ActiveLaunch: test.active, Event: AutomationEvent{Type: workflowmodel.StageEntered, StageVersion: 1, ActorID: "actor"}}}
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

func TestStageEntryPreservesExternalStartupContext(t *testing.T) {
	definition := workflowmodel.DefaultDefinition("Default")
	service := NewWorkflowAutomationService(nil, nil, nil, WorkflowRuleEvaluator{})
	effects, err := service.evaluateEvent(AutomationEventSnapshot{Workflow: definition, Event: AutomationEvent{ID: "entry", Type: workflowmodel.StageEntered, StageVersion: 2, ActorID: "admin", TaskID: "task", PromptText: "Inspect https://github.com/owner/repo/pull/4"}, StageID: "in_progress", StageVersion: 2, AssigneeAgentID: "agent", WorkState: taskmodel.WorkStateIdle})
	if err != nil {
		t.Fatal(err)
	}
	if effects.Persistence.Execution == nil || effects.Persistence.Execution.Options.PromptText != "Inspect https://github.com/owner/repo/pull/4" || effects.Persistence.Execution.Options.SessionMode != "new" {
		t.Fatalf("startup context lost: %+v", effects.Persistence.Execution)
	}
}
