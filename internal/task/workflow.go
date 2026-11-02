package task

import (
	"context"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
)

type TransitionRepository interface {
	TransitionTask(context.Context, string, string, string, string, TransitionInput) (Task, error)
	UpdateWorkflowTask(context.Context, string, string, string, string, TaskInput) (Task, error)
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
