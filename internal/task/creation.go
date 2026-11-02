package task

import (
	"context"
	"fmt"

	"github.com/josemiguelmelo/orkest/internal/core/ids"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/workflow"
)

type (
	TaskCreationSnapshot struct{ Workflow workflow.Definition }
	TaskCreationEffects  struct {
		Input          TaskInput
		EmitStageEntry bool
	}
)

func (s *TaskService) Create(ctx context.Context, userID, orgID, workspaceID string, input TaskInput) (Task, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return Task{}, err
	}
	if input.WorkflowID == "" {
		return Task{}, modelerrors.ErrInvalid
	}
	if err := NormalizeTaskContent(&input); err != nil {
		return Task{}, err
	}
	if err := ValidateWorkOptions(input.SessionMode, input.PromptText, input.CommandID); err != nil {
		return Task{}, err
	}
	input.ActorID = userID
	id, err := ids.NewUUID()
	if err != nil {
		return Task{}, fmt.Errorf("generate task ID: %w", err)
	}
	return s.creations.WithTaskCreation(ctx, id, workspaceID, input, s.planTaskCreation)
}

func (s *TaskService) planTaskCreation(snapshot TaskCreationSnapshot, input TaskInput) (TaskCreationEffects, error) {
	if input.Status != "" && input.Status != snapshot.Workflow.InitialStageID {
		return TaskCreationEffects{}, modelerrors.ErrInvalid
	}
	input.Status = snapshot.Workflow.InitialStageID
	stage, ok := snapshot.Workflow.Stage(input.Status)
	if !ok {
		return TaskCreationEffects{}, modelerrors.ErrInvalid
	}
	if stage.ResponsibleAgentID != "" {
		input.AssigneeAgentID = stage.ResponsibleAgentID
	}

	return TaskCreationEffects{Input: input, EmitStageEntry: true}, nil
}
