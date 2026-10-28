package task

import (
	"context"

	"github.com/josemiguelmelo/orkest/internal/workspace"
)

type Repository interface {
	Tasks(context.Context, string, string) ([]Task, error)
	Task(context.Context, string, string, string) (Task, error)
	CreateTask(context.Context, string, string, TaskInput) (Task, error)
	UpdateTask(context.Context, string, string, string, TaskInput) (Task, error)
	UpdateTaskIfStatus(context.Context, string, string, string, string, TaskInput) (Task, error)
	Comments(context.Context, string, string, string) ([]Comment, error)
	CreateComment(context.Context, string, string, string, string, string) (Comment, error)
}

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error)
}
