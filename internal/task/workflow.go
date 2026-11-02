package task

import (
	"context"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
)

type TransitionRepository interface {
	TransitionTask(context.Context, string, string, string, string, TaskInput) (Task, error)
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
	return s.transitions.TransitionTask(ctx, userID, orgID, workspaceID, taskID, in)
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
	current, err := s.Get(ctx, userID, orgID, workspaceID, taskID)
	if err != nil {
		return Task{}, err
	}
	target := current.Status
	if in.Action == "move" {
		target = in.StageID
	}
	input := TaskInput{Title: current.Title, Description: current.Description, Status: target, AssigneeAgentID: current.AssigneeAgentID, Labels: current.Labels, StageVersion: in.StageVersion, StartWork: in.Action == "start", CompleteStage: in.Action == "complete", CommandID: in.CommandID, SessionMode: in.SessionMode, PromptText: in.PromptText}
	for _, r := range current.Repositories {
		input.RepositoryIDs = append(input.RepositoryIDs, r.ID)
	}
	return s.updateWorkflowTask(ctx, userID, orgID, workspaceID, taskID, input)
}
