package task

import (
	"context"
	"errors"

	"github.com/josemiguelmelo/orkest/internal/workflow"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
)

type TransitionRepository interface {
	TransitionTask(context.Context, string, string, string, string, TransitionInput) (Task, error)
	UpdateWorkflowTask(context.Context, string, string, string, string, TaskInput) (Task, error)
	WithWorkflowEvent(context.Context, func(WorkflowEventSnapshot) (WorkflowEventDecision, error)) (bool, error)
}

func (s *Service) updateWorkflowTask(ctx context.Context, userID, orgID, workspaceID, taskID string, in TaskInput) (Task, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return Task{}, err
	}
	if err := normalizeTaskContent(&in); err != nil {
		return Task{}, err
	}
	if in.Status == "" {
		return Task{}, modelerrors.ErrInvalid
	}
	return s.transitions.UpdateWorkflowTask(ctx, userID, orgID, workspaceID, taskID, in)
}

type TransitionInput struct {
	StageID      string `json:"stageId"`
	StageVersion int64  `json:"stageVersion"`
	Action       string `json:"action"`
	CommandID    string `json:"commandId"`
	SessionMode  string `json:"sessionMode"`
	PromptText   string `json:"promptText"`
}

func (s *Service) Transition(ctx context.Context, userID, orgID, workspaceID, taskID string, in TransitionInput) (Task, error) {
	if in.CommandID == "" || in.StageVersion < 1 || (in.Action != "move" && in.Action != "start" && in.Action != "complete") {
		return Task{}, modelerrors.ErrInvalid
	}
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return Task{}, err
	}
	if in.Action == "move" && (in.StageID == "" || len(in.StageID) > 100) {
		return Task{}, modelerrors.ErrInvalid
	}
	if in.Action != "move" {
		in.StageID = ""
	}
	if err := validateExecutionOptions(in.SessionMode, in.PromptText, in.CommandID); err != nil {
		return Task{}, err
	}
	return s.transitions.TransitionTask(ctx, userID, orgID, workspaceID, taskID, in)
}

// ProcessWorkflowEvent processes and acknowledges one durable event atomically.
func (s *Service) ProcessWorkflowEvent(ctx context.Context) (bool, error) {
	return s.transitions.WithWorkflowEvent(ctx, s.evaluateWorkflowEvent)
}

// WorkflowEventSnapshot contains persistence-independent facts read under the
// repository's event, workflow and task locks. The callback performs no I/O.
type WorkflowEventSnapshot struct {
	Workflow                        workflow.Definition
	Event                           WorkflowEvent
	StageID, AssigneeAgentID, RunID string
	StageVersion                    int64
	WorkState                       WorkState
	ActiveLaunch                    bool
}

type WorkflowEvent struct {
	Type           workflow.EventType
	StageVersion   int64
	RunID, ActorID string
}

// A nil Plan and empty WorkState acknowledge an obsolete or superseded event.
type WorkflowEventDecision struct {
	Plan            *workflow.Plan
	WorkState       WorkState
	RuleID, ActorID string
}

func (s *Service) evaluateWorkflowEvent(snapshot WorkflowEventSnapshot) (WorkflowEventDecision, error) {
	e := snapshot.Event
	if snapshot.StageVersion != e.StageVersion {
		return WorkflowEventDecision{}, nil
	}
	switch e.Type {
	case workflow.RunSucceeded, workflow.RunFailed:
		if snapshot.RunID != e.RunID || snapshot.WorkState == WorkStateCompleted || snapshot.WorkState == WorkStateFailed {
			return WorkflowEventDecision{}, nil
		}
	case workflow.StageEntered:
		if snapshot.WorkState != WorkStateIdle || snapshot.RunID != "" {
			return WorkflowEventDecision{}, nil
		}
	case workflow.StageCompleted:
		if snapshot.WorkState != WorkStateCompleted || snapshot.RunID != e.RunID {
			return WorkflowEventDecision{}, nil
		}
	}
	if e.Type == workflow.RunFailed {
		return WorkflowEventDecision{WorkState: WorkStateFailed}, nil
	}
	plan, err := s.evaluator.Evaluate(snapshot.Workflow, workflow.Plan{StageID: snapshot.StageID, AssigneeAgentID: snapshot.AssigneeAgentID}, workflow.Event{Type: e.Type})
	if errors.Is(err, modelerrors.ErrConflict) {
		return WorkflowEventDecision{RuleID: plan.RuleID, WorkState: WorkStateFailed}, nil
	}
	if err != nil {
		return WorkflowEventDecision{}, err
	}
	if plan.Start && snapshot.ActiveLaunch {
		return WorkflowEventDecision{RuleID: plan.RuleID}, nil
	}
	actor := e.ActorID
	if actor == "" {
		actor = snapshot.Workflow.ConfiguredBy
	}
	return WorkflowEventDecision{Plan: &plan, RuleID: plan.RuleID, ActorID: actor}, nil
}
