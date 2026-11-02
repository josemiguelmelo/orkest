package workflowautomation

import (
	"errors"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/task"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	definition "github.com/josemiguelmelo/orkest/internal/workflow"
)

func transitionSnapshot() TransitionSnapshot {
	d := definition.DefaultDefinition("Board")
	d.Stages[1].ResponsibleAgentID = "stage-agent"
	return TransitionSnapshot{Workflow: d, Task: task.Task{Status: "todo", StageVersion: 4, AssigneeAgentID: "agent", SourceProvider: "manual", Title: "Original", Description: "Original description", Labels: []string{"original"}, StageWorkState: task.WorkStateIdle}}
}

func TestTransitionPolicyConflicts(t *testing.T) {
	cases := []struct {
		name   string
		change func(*TransitionSnapshot, *TransitionCommand)
		want   error
	}{
		{"stale version", func(s *TransitionSnapshot, c *TransitionCommand) { c.Input.StageVersion-- }, modelerrors.ErrConflict},
		{"queued move", func(s *TransitionSnapshot, c *TransitionCommand) { s.Task.StageWorkState = task.WorkStateQueued }, modelerrors.ErrConflict},
		{"running move", func(s *TransitionSnapshot, c *TransitionCommand) { s.Task.StageWorkState = task.WorkStateRunning }, modelerrors.ErrConflict},
		{"active launch", func(s *TransitionSnapshot, c *TransitionCommand) { s.ActiveLaunch = true }, modelerrors.ErrConflict},
		{"closed task", func(s *TransitionSnapshot, c *TransitionCommand) { s.Task.Status = "done" }, modelerrors.ErrConflict},
		{"unknown destination", func(s *TransitionSnapshot, c *TransitionCommand) { c.Input.StageID = "missing" }, modelerrors.ErrInvalid},
		{"unknown action", func(s *TransitionSnapshot, c *TransitionCommand) { c.Input.Action = "missing" }, modelerrors.ErrInvalid},
		{"unassigned start", func(s *TransitionSnapshot, c *TransitionCommand) {
			s.Task.AssigneeAgentID = ""
			c.Input.Action = "start"
		}, modelerrors.ErrConflict},
		{"closed start", func(s *TransitionSnapshot, c *TransitionCommand) { s.Task.Status = "done"; c.Input.Action = "start" }, modelerrors.ErrConflict},
		{"queued start", func(s *TransitionSnapshot, c *TransitionCommand) {
			s.Task.StageWorkState = task.WorkStateQueued
			c.Input.Action = "start"
		}, modelerrors.ErrConflict},
		{"running completion", func(s *TransitionSnapshot, c *TransitionCommand) {
			s.Task.StageWorkState = task.WorkStateRunning
			c.Input.Action = "complete"
		}, modelerrors.ErrConflict},
		{"active reassignment", func(s *TransitionSnapshot, c *TransitionCommand) {
			s.ActiveLaunch = true
			other := "other"
			c.AssigneeAgentID = &other
			c.Input.StageID = s.Task.Status
		}, modelerrors.ErrConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := transitionSnapshot()
			command := TransitionCommand{Input: TransitionRequest{Action: "move", StageID: "in_progress", StageVersion: 4}}
			tc.change(&snapshot, &command)
			if _, err := PlanTaskTransition(snapshot, command); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestTransitionMovementAndStartEffects(t *testing.T) {
	snapshot := transitionSnapshot()
	decision, err := PlanTaskTransition(snapshot, TransitionCommand{Input: TransitionRequest{Action: "move", StageID: "in_progress", StageVersion: 4}})
	if err != nil || !decision.Moved || decision.StageVersion != 5 || decision.Plan.StageID != "in_progress" || decision.Plan.AssigneeAgentID != "stage-agent" || decision.Plan.Start || decision.CompleteStage {
		t.Fatalf("movement effects: %+v, %v", decision, err)
	}
	// Entry automation is asynchronous, even when the destination has a start rule.
	snapshot.Task.Status = "in_review"
	decision, err = PlanTaskTransition(snapshot, TransitionCommand{Input: TransitionRequest{Action: "start", StageID: "ignored", StageVersion: 4, CommandID: "command", SessionMode: "continue", PromptText: "revise"}})
	if err != nil || !decision.Plan.Start || decision.Moved || decision.StageVersion != 4 || decision.Input.StageID != "" || decision.Input.SessionMode != "continue" || decision.Input.PromptText != "revise" {
		t.Fatalf("start effects: %+v, %v", decision, err)
	}
}

func TestTransitionCompletionDefersRules(t *testing.T) {
	snapshot := transitionSnapshot()
	snapshot.Task.Status = "in_progress"
	decision, err := PlanTaskTransition(snapshot, TransitionCommand{Input: TransitionRequest{Action: "complete", StageVersion: 4}})
	if err != nil || !decision.CompleteStage || !decision.Plan.Completed || decision.Moved || decision.Plan.StageID != "in_progress" || decision.Plan.Start || decision.StageVersion != 4 {
		t.Fatalf("completion must emit an event without evaluating movement rules: %+v, %v", decision, err)
	}
}

func TestTransitionReplayDefersToPersistence(t *testing.T) {
	snapshot := transitionSnapshot()
	snapshot.RecordedCommand = true
	snapshot.Task.Status = "done"
	snapshot.Task.StageWorkState = task.WorkStateRunning
	snapshot.ActiveLaunch = true
	decision, err := PlanTaskTransition(snapshot, TransitionCommand{Input: TransitionRequest{Action: "start", StageID: "ignored", StageVersion: 1, CommandID: "recorded"}})
	if err != nil || decision.Input.StageID != "" || decision.Input.StageVersion != 1 || decision.Plan.Start || decision.Moved || decision.CompleteStage {
		t.Fatalf("replay must return only canonical input for repository comparison: %+v, %v", decision, err)
	}
}
