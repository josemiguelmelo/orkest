package task

import (
	"context"
	"fmt"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	"github.com/josemiguelmelo/orkest/internal/server/workflow"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
)

type (
	TaskCreationSnapshot struct {
		Workflow workflowmodel.Definition
		TaskID   string
	}
	TaskCreationEffects struct {
		Input          taskmodel.TaskInput
		EmitStageEntry bool
		Events         []workflow.EventIntent
	}
)

func (s *TaskService) Create(ctx context.Context, userID, orgID, workspaceID string, input taskmodel.TaskInput) (taskmodel.Task, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return taskmodel.Task{}, err
	}
	if input.WorkflowID == "" {
		return taskmodel.Task{}, modelerrors.ErrInvalid
	}
	if err := NormalizeTaskContent(&input); err != nil {
		return taskmodel.Task{}, err
	}
	if err := ValidateWorkOptions(input.SessionMode, input.PromptText, input.CommandID); err != nil {
		return taskmodel.Task{}, err
	}
	input.ActorID = userID
	id, err := ids.NewUUID()
	if err != nil {
		return taskmodel.Task{}, fmt.Errorf("generate task ID: %w", err)
	}
	return s.creations.WithTaskCreation(ctx, id, workspaceID, input, s.planTaskCreation)
}

func (s *TaskService) planTaskCreation(snapshot TaskCreationSnapshot, input taskmodel.TaskInput) (TaskCreationEffects, error) {
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

	return TaskCreationEffects{Input: input, EmitStageEntry: true, Events: []workflow.EventIntent{workflow.StageEvent(snapshot.TaskID, 1, workflowmodel.StageEntered, "", input.CommandID, "")}}, nil
}
