package task

import (
	"context"

	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type TaskRepository interface {
	Tasks(context.Context, string, string) ([]taskmodel.Task, error)
	Task(context.Context, string, string, string) (taskmodel.Task, error)
	Comments(context.Context, string, string, string) ([]taskmodel.Comment, error)
	CreateComment(context.Context, string, string, string, string, string) (taskmodel.Comment, error)
}

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error)
}

// TaskCreationRepository commits the task and its entry event under the workflow lock.
type TaskCreationRepository interface {
	WithTaskCreation(context.Context, string, string, taskmodel.TaskInput, func(TaskCreationSnapshot, taskmodel.TaskInput) (TaskCreationEffects, error)) (taskmodel.Task, error)
}
