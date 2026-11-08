package workflowautomation

import (
	"context"
	"errors"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/task"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
)

// ProcessNextEvent processes and acknowledges one durable event atomically.
func (s *WorkflowAutomationService) ProcessNextEvent(ctx context.Context) (bool, error) {
	return s.events.WithAutomationEvent(ctx, s.evaluateEvent)
}

// AutomationEventSnapshot contains persistence-independent facts read under the
// repository's event, workflow and task locks. The callback performs no I/O.
type AutomationEventSnapshot struct {
	Workflow                        workflowmodel.Definition
	Event                           AutomationEvent
	StageID, AssigneeAgentID, RunID string
	StageVersion                    int64
	WorkState                       taskmodel.WorkState
	ActiveLaunch                    bool
}

type AutomationEvent struct {
	PromptText                             string
	ID, CorrelationID, WorkspaceID, TaskID string
	Type                                   workflowmodel.EventType
	StageVersion                           int64
	RunID, ActorID                         string
}

// A nil StageTransitionPlan and empty taskmodel.WorkState acknowledge an obsolete or superseded event.
type AutomationEventEffects struct {
	Plan                 *StageTransitionPlan
	Moved                bool
	Persistence          task.WorkflowPersistenceEffects
	ConfigurationVersion int64
	AcknowledgeID        string
	WorkState            taskmodel.WorkState
	RuleID, ActorID      string
}

func (s *WorkflowAutomationService) evaluateEvent(snapshot AutomationEventSnapshot) (AutomationEventEffects, error) {
	effects, err := s.planEvent(snapshot)
	effects.ConfigurationVersion = snapshot.Workflow.Version
	effects.AcknowledgeID = snapshot.Event.ID
	return effects, err
}

func (s *WorkflowAutomationService) planEvent(snapshot AutomationEventSnapshot) (AutomationEventEffects, error) {
	e := snapshot.Event
	if snapshot.StageVersion != e.StageVersion {
		return AutomationEventEffects{}, nil
	}
	switch e.Type {
	case workflowmodel.RunSucceeded, workflowmodel.RunFailed:
		if snapshot.RunID != e.RunID || snapshot.WorkState == taskmodel.WorkStateCompleted || snapshot.WorkState == taskmodel.WorkStateFailed {
			return AutomationEventEffects{}, nil
		}
	case workflowmodel.StageEntered:
		if snapshot.WorkState != taskmodel.WorkStateIdle || snapshot.RunID != "" {
			return AutomationEventEffects{}, nil
		}
	case workflowmodel.StageCompleted:
		if snapshot.WorkState != taskmodel.WorkStateCompleted || snapshot.RunID != e.RunID {
			return AutomationEventEffects{}, nil
		}
	}
	if e.Type == workflowmodel.RunFailed {
		return AutomationEventEffects{WorkState: taskmodel.WorkStateFailed}, nil
	}
	plan, err := s.evaluator.Evaluate(snapshot.Workflow, StageTransitionPlan{StageID: snapshot.StageID, AssigneeAgentID: snapshot.AssigneeAgentID}, Event{Type: e.Type})
	if errors.Is(err, modelerrors.ErrConflict) {
		return AutomationEventEffects{RuleID: plan.RuleID, WorkState: taskmodel.WorkStateFailed}, nil
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
		return AutomationEventEffects{RuleID: plan.RuleID, WorkState: taskmodel.WorkStateFailed}, nil
	}
	moved := plan.StageID != snapshot.StageID
	version := snapshot.StageVersion
	if moved {
		version++
	}
	persistence, err := task.PlanWorkflowPersistence(e.TaskID, e.RunID, task.TaskWorkflowEffects{Command: task.WorkflowCommand{CommandID: e.ID + ":start", SessionMode: "new", PromptText: e.PromptText}, Start: plan.Start, Moved: moved, Completed: plan.Completed, StageVersion: version}, e.ID, e.CorrelationID, "")
	// Automation commands do not add a separate user-transition history row.
	persistence.History = nil
	return AutomationEventEffects{Plan: &plan, RuleID: plan.RuleID, ActorID: actor, Moved: moved, Persistence: persistence}, err
}
