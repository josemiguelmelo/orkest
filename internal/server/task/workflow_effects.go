package task

import (
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/workflow"
)

// WorkflowPersistenceEffects contains all outbox and history effects of a decision.
// Repositories only map these values and enforce their locked version/uniqueness.
type WorkflowPersistenceEffects struct {
	Events       []workflow.EventIntent
	Execution    *WorkflowExecutionIntent
	History      *WorkflowCommand
	WorkState    WorkState
	StageVersion int64
}

type WorkflowExecutionIntent struct {
	ID      string
	Options WorkflowExecutionOptions
}

type WorkflowExecutionOptions struct {
	Action       string `json:"action"`
	CommandID    string `json:"commandId"`
	StageVersion int64  `json:"stageVersion"`
	SessionMode  string `json:"sessionMode"`
	PromptText   string `json:"promptText"`
}

func PlanWorkflowPersistence(taskID, runID string, effects TaskWorkflowEffects, cause, correlation, candidateID string) (WorkflowPersistenceEffects, error) {
	out := WorkflowPersistenceEffects{StageVersion: effects.StageVersion}
	in := effects.Command
	if effects.Completed && in.Action == "complete" {
		out.Events = append(out.Events, workflow.StageEvent(taskID, effects.StageVersion, workflow.StageCompleted, runID, in.CommandID, in.CommandID))
	}
	if effects.Moved {
		out.Events = append(out.Events, workflow.StageEvent(taskID, effects.StageVersion, workflow.StageEntered, "", cause, correlation))
	}
	if effects.Start {
		id := in.CommandID
		if id == "" {
			id = candidateID
		}
		if id == "" {
			return WorkflowPersistenceEffects{}, modelerrors.ErrInvalid
		}

		out.Execution = &WorkflowExecutionIntent{ID: id, Options: WorkflowExecutionOptions{Action: "start", CommandID: id, StageVersion: in.StageVersion, SessionMode: in.SessionMode, PromptText: in.PromptText}}
		out.WorkState = WorkStateQueued
	} else if effects.Completed {
		out.WorkState = WorkStateCompleted
	}
	if in.CommandID != "" && (!effects.Start || in.Action == "move") {
		out.History = &in
	}
	return out, nil
}
