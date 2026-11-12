package task

import (
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	"github.com/josemiguelmelo/orkest/internal/server/workflow"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
)

// WorkflowPersistenceEffects contains all outbox and history effects of a decision.
// Repositories only map these values and enforce their locked version/uniqueness.
type WorkflowPersistenceEffects struct {
	Events       []workflow.EventIntent
	Execution    *WorkflowExecutionIntent
	History      *WorkflowCommand
	WorkState    taskmodel.WorkState
	StageVersion int64
}

type WorkflowExecutionIntent struct {
	ID      string
	Options WorkflowExecutionOptions
}

type WorkflowExecutionOptions struct {
	Integration        *workflowmodel.IntegrationEvent `json:"integration,omitempty"`
	Action             string                          `json:"action"`
	CommandID          string                          `json:"commandId"`
	StageVersion       int64                           `json:"stageVersion"`
	SessionMode        string                          `json:"sessionMode"`
	PromptText         string                          `json:"promptText"`
	ExpectedAssigneeID string                          `json:"expectedAssigneeId,omitempty"`
	GuardAssignment    bool                            `json:"guardAssignment,omitempty"`
}

func PlanWorkflowPersistence(taskID, runID string, effects TaskWorkflowEffects, cause, correlation, candidateID string) (WorkflowPersistenceEffects, error) {
	out := WorkflowPersistenceEffects{StageVersion: effects.StageVersion}
	in := effects.Command
	if effects.Completed && in.Action == "complete" {
		out.Events = append(out.Events, workflow.StageEvent(taskID, effects.StageVersion, workflowmodel.StageCompleted, runID, in.CommandID, in.CommandID))
	}
	if effects.Moved {
		out.Events = append(out.Events, workflow.StageEvent(taskID, effects.StageVersion, workflowmodel.StageEntered, "", cause, correlation))
	}
	if effects.Start {
		id := in.CommandID
		if id == "" {
			id = candidateID
		}
		if id == "" {
			return WorkflowPersistenceEffects{}, modelerrors.ErrInvalid
		}

		out.Execution = &WorkflowExecutionIntent{ID: id, Options: WorkflowExecutionOptions{Action: "start", CommandID: id, StageVersion: in.StageVersion, SessionMode: in.SessionMode, PromptText: in.PromptText, ExpectedAssigneeID: in.ExpectedAssigneeID, GuardAssignment: in.GuardAssignment}}
		out.WorkState = taskmodel.WorkStateQueued
	} else if effects.Completed {
		out.WorkState = taskmodel.WorkStateCompleted
	}
	if in.CommandID != "" && (!effects.Start || in.Action == "move") {
		out.History = &in
	}
	return out, nil
}
