package task

import (
	"context"

	"github.com/josemiguelmelo/orkest/internal/server/workspace"
)

type TaskRepository interface {
	Tasks(context.Context, string, string) ([]Task, error)
	Task(context.Context, string, string, string) (Task, error)
	Comments(context.Context, string, string, string) ([]Comment, error)
	CreateComment(context.Context, string, string, string, string, string) (Comment, error)
}

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error)
}

// TaskCreationRepository commits the task and its entry event under the workflow lock.
type TaskCreationRepository interface {
	WithTaskCreation(context.Context, string, string, TaskInput, func(TaskCreationSnapshot, TaskInput) (TaskCreationEffects, error)) (Task, error)
}
