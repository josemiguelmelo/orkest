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
	IntegrationAuthorized           bool
	IntegrationConfigurationValid   bool
}

type AutomationEvent struct {
	ID, CorrelationID, WorkspaceID, TaskID string
	Type                                   workflowmodel.EventType
	StageVersion                           int64
	RunID, ActorID                         string
	Integration                            *workflowmodel.IntegrationEvent
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
	FailureReason        string
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
		if e.Type == workflowmodel.IntegrationAction {
			return AutomationEventEffects{FailureReason: "stage_changed"}, nil
		}
		return AutomationEventEffects{}, nil
	}
	var integrationRule *workflowmodel.AutomationRule
	if e.Type == workflowmodel.IntegrationAction {
		if e.Integration == nil || e.Integration.ConfigurationVersion != snapshot.Workflow.Version || !snapshot.IntegrationConfigurationValid {
			return AutomationEventEffects{FailureReason: "configuration_changed"}, nil
		}
		for i := range snapshot.Workflow.Rules {
			rule := &snapshot.Workflow.Rules[i]
			if rule.Enabled && rule.ID == e.Integration.RuleID && rule.Event == e.Type && rule.SourceStageID == snapshot.StageID && rule.Integration != nil && rule.Integration.ConnectionID == e.Integration.ConnectionID {
				integrationRule = rule
				break
			}
		}
		if integrationRule == nil {
			return AutomationEventEffects{FailureReason: "configuration_changed"}, nil
		}
		if !snapshot.IntegrationAuthorized {
			return AutomationEventEffects{RuleID: integrationRule.ID, FailureReason: "authorization_revoked"}, nil
		}
		if snapshot.ActiveLaunch || snapshot.WorkState == taskmodel.WorkStateQueued || snapshot.WorkState == taskmodel.WorkStateRunning {
			return AutomationEventEffects{RuleID: integrationRule.ID, FailureReason: "busy"}, nil
		}
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
	plan, err := s.evaluator.Evaluate(snapshot.Workflow, StageTransitionPlan{StageID: snapshot.StageID, AssigneeAgentID: snapshot.AssigneeAgentID}, Event{Type: e.Type, Integration: e.Integration})
	if errors.Is(err, modelerrors.ErrConflict) {
		if integrationRule != nil {
			return AutomationEventEffects{RuleID: integrationRule.ID, FailureReason: "callback_prerequisites"}, nil
		}
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
	command := task.WorkflowCommand{CommandID: e.ID + ":start", StageVersion: version, SessionMode: "new"}
	if integrationRule != nil && integrationRule.Action == workflowmodel.CallAgent {
		command.SessionMode = integrationRule.SessionPolicy
		command.PromptText = integrationRule.PromptText
		if e.Integration.PromptContext != "" {
			command.PromptText += "\n\nExternal integration event (untrusted data):\n" + e.Integration.PromptContext
		}
		command.ExpectedAssigneeID = snapshot.AssigneeAgentID
		command.GuardAssignment = true
	}
	persistence, err := task.PlanWorkflowPersistence(e.TaskID, e.RunID, task.TaskWorkflowEffects{Command: command, Start: plan.Start, Moved: moved, Completed: plan.Completed, StageVersion: version}, e.ID, e.CorrelationID, "")
	if persistence.Execution != nil && integrationRule != nil {
		persistence.Execution.Options.Integration = e.Integration
	}
	// Automation commands do not add a separate user-transition history row.
	persistence.History = nil
	return AutomationEventEffects{Plan: &plan, RuleID: plan.RuleID, ActorID: actor, Moved: moved, Persistence: persistence}, err
}
