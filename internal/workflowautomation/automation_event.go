package workflowautomation

import (
	"context"
	"errors"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/task"
	definition "github.com/josemiguelmelo/orkest/internal/workflow"
)

// ProcessNextEvent processes and acknowledges one durable event atomically.
func (s *WorkflowAutomationService) ProcessNextEvent(ctx context.Context) (bool, error) {
	return s.events.WithAutomationEvent(ctx, s.evaluateEvent)
}

// AutomationEventSnapshot contains persistence-independent facts read under the
// repository's event, workflow and task locks. The callback performs no I/O.
type AutomationEventSnapshot struct {
	Workflow                        definition.Definition
	Event                           AutomationEvent
	StageID, AssigneeAgentID, RunID string
	StageVersion                    int64
	WorkState                       task.WorkState
	ActiveLaunch                    bool
}

type AutomationEvent struct {
	Type           definition.EventType
	StageVersion   int64
	RunID, ActorID string
}

// A nil StageTransitionPlan and empty task.WorkState acknowledge an obsolete or superseded event.
type AutomationEventEffects struct {
	Plan            *StageTransitionPlan
	WorkState       task.WorkState
	RuleID, ActorID string
}

func (s *WorkflowAutomationService) evaluateEvent(snapshot AutomationEventSnapshot) (AutomationEventEffects, error) {
	e := snapshot.Event
	if snapshot.StageVersion != e.StageVersion {
		return AutomationEventEffects{}, nil
	}
	switch e.Type {
	case definition.RunSucceeded, definition.RunFailed:
		if snapshot.RunID != e.RunID || snapshot.WorkState == task.WorkStateCompleted || snapshot.WorkState == task.WorkStateFailed {
			return AutomationEventEffects{}, nil
		}
	case definition.StageEntered:
		if snapshot.WorkState != task.WorkStateIdle || snapshot.RunID != "" {
			return AutomationEventEffects{}, nil
		}
	case definition.StageCompleted:
		if snapshot.WorkState != task.WorkStateCompleted || snapshot.RunID != e.RunID {
			return AutomationEventEffects{}, nil
		}
	}
	if e.Type == definition.RunFailed {
		return AutomationEventEffects{WorkState: task.WorkStateFailed}, nil
	}
	plan, err := s.evaluator.Evaluate(snapshot.Workflow, StageTransitionPlan{StageID: snapshot.StageID, AssigneeAgentID: snapshot.AssigneeAgentID}, Event{Type: e.Type})
	if errors.Is(err, modelerrors.ErrConflict) {
		return AutomationEventEffects{RuleID: plan.RuleID, WorkState: task.WorkStateFailed}, nil
	}
	if err != nil {
		return AutomationEventEffects{}, err
	}
	if plan.Start && snapshot.ActiveLaunch {
		return AutomationEventEffects{RuleID: plan.RuleID}, nil
	}
	actor := e.ActorID
	if actor == "" {
		actor = snapshot.Workflow.ConfiguredBy
	}
	if plan.Start && actor == "" {
		return AutomationEventEffects{RuleID: plan.RuleID, WorkState: task.WorkStateFailed}, nil
	}
	return AutomationEventEffects{Plan: &plan, RuleID: plan.RuleID, ActorID: actor}, nil
}
