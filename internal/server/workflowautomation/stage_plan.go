package workflowautomation

import (
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/workflow"
)

type Trigger string

const (
	Enter     Trigger = "enter"
	Start     Trigger = "start"
	Succeeded Trigger = "succeeded"
	Complete  Trigger = "complete"
)

type StageTransitionPlan struct {
	RuleID          string
	StageID         string
	AssigneeAgentID string
	Start           bool
	Completed       bool
}

// PlanStageTransition has no effects; callers persist its result atomically.
func PlanStageTransition(d workflow.Definition, currentID, targetID, assignee string, trigger Trigger) (StageTransitionPlan, error) {
	if _, ok := d.Stage(currentID); !ok {
		return StageTransitionPlan{}, modelerrors.ErrInvalid
	}
	plan := StageTransitionPlan{StageID: currentID, AssigneeAgentID: assignee}
	switch trigger {
	case Succeeded, Complete:
		event := workflow.RunSucceeded
		if trigger == Complete {
			event = workflow.StageCompleted
		}
		return (WorkflowRuleEvaluator{}).Evaluate(d, plan, Event{Type: event})
	case Start:
		if currentID == d.ClosedStageID || assignee == "" {
			return StageTransitionPlan{}, modelerrors.ErrConflict
		}
		plan.Start = true
		return plan, nil
	case Enter:
	default:
		return StageTransitionPlan{}, modelerrors.ErrInvalid
	}
	if currentID == d.ClosedStageID && targetID != currentID {
		return StageTransitionPlan{}, modelerrors.ErrConflict
	}
	next, ok := d.Stage(targetID)
	if !ok {
		return StageTransitionPlan{}, modelerrors.ErrInvalid
	}
	plan.StageID = targetID
	if next.ResponsibleAgentID != "" {
		plan.AssigneeAgentID = next.ResponsibleAgentID
	}
	return plan, nil
}
