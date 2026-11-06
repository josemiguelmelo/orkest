package workflowautomation

import (
	"context"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/task"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

// Service owns task transition policy and durable automation event decisions.
type WorkflowAutomationService struct {
	transitions TaskTransitionRepository
	events      AutomationEventRepository
	access      WorkspaceAccess
	evaluator   AutomationEvaluator
}

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error)
}

func NewWorkflowAutomationService(transitions TaskTransitionRepository, events AutomationEventRepository, access WorkspaceAccess, evaluator AutomationEvaluator) *WorkflowAutomationService {
	return &WorkflowAutomationService{transitions: transitions, events: events, access: access, evaluator: evaluator}
}

func (s *WorkflowAutomationService) authorize(ctx context.Context, userID, orgID, workspaceID string) error {
	w, err := s.access.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return err
	}
	if w.Role == "viewer" {
		return modelerrors.ErrForbidden
	}
	return nil
}

func (s *WorkflowAutomationService) Transition(ctx context.Context, userID, orgID, workspaceID, taskID string, in TransitionRequest) (taskmodel.Task, error) {
	if in.CommandID == "" || in.StageVersion < 1 || (in.Action != "move" && in.Action != "start" && in.Action != "complete") {
		return taskmodel.Task{}, modelerrors.ErrInvalid
	}
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return taskmodel.Task{}, err
	}
	if in.Action == "move" && (in.StageID == "" || len(in.StageID) > 100) {
		return taskmodel.Task{}, modelerrors.ErrInvalid
	}
	if in.Action != "move" {
		in.StageID = ""
	}
	if err := task.ValidateWorkOptions(in.SessionMode, in.PromptText, in.CommandID); err != nil {
		return taskmodel.Task{}, err
	}
	return s.transitions.WithTaskTransition(ctx, userID, orgID, workspaceID, taskID, TransitionCommand{Input: in}, PlanTaskTransition)
}
