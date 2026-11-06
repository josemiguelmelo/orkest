package task

import (
	"context"
	"fmt"
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
)

type TaskService struct {
	repository TaskRepository
	workspaces WorkspaceAccess
	creations  TaskCreationRepository
	updates    TaskUpdateRepository
	workflow   TaskWorkflowPlanner
}

func NewTaskService(repository TaskRepository, creations TaskCreationRepository, updates TaskUpdateRepository, workflow TaskWorkflowPlanner, workspaces WorkspaceAccess) *TaskService {
	return &TaskService{repository: repository, creations: creations, updates: updates, workflow: workflow, workspaces: workspaces}
}

func (s *TaskService) List(ctx context.Context, userID, orgID, workspaceID string) ([]taskmodel.Task, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repository.Tasks(ctx, orgID, workspaceID)
}

func (s *TaskService) Get(ctx context.Context, userID, orgID, workspaceID, taskID string) (taskmodel.Task, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return taskmodel.Task{}, err
	}
	return s.repository.Task(ctx, orgID, workspaceID, taskID)
}

// AuthorizeExecution checks task visibility and whether the caller may interact with its run.
func (s *TaskService) AuthorizeExecution(ctx context.Context, userID, orgID, workspaceID, taskID string, mutate bool) error {
	if _, err := s.repository.Task(ctx, orgID, workspaceID, taskID); err != nil {
		return err
	}
	w, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return err
	}
	if mutate && w.Role == "viewer" {
		return modelerrors.ErrForbidden
	}
	return nil
}

func (s *TaskService) Comments(ctx context.Context, userID, orgID, workspaceID, taskID string) ([]taskmodel.Comment, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	if _, err := s.repository.Task(ctx, orgID, workspaceID, taskID); err != nil {
		return nil, err
	}
	return s.repository.Comments(ctx, orgID, workspaceID, taskID)
}

func (s *TaskService) AddComment(ctx context.Context, userID, orgID, workspaceID, taskID, body string) (taskmodel.Comment, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return taskmodel.Comment{}, err
	}
	if _, err := s.repository.Task(ctx, orgID, workspaceID, taskID); err != nil {
		return taskmodel.Comment{}, err
	}
	body = strings.TrimSpace(body)
	if body == "" || len(body) > 20000 {
		return taskmodel.Comment{}, modelerrors.ErrInvalid
	}
	id, err := ids.NewUUID()
	if err != nil {
		return taskmodel.Comment{}, fmt.Errorf("generate comment ID: %w", err)
	}
	return s.repository.CreateComment(ctx, id, workspaceID, taskID, userID, body)
}

func (s *TaskService) authorize(ctx context.Context, userID, orgID, workspaceID string) error {
	w, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return err
	}
	if w.Role == "viewer" {
		return modelerrors.ErrForbidden
	}
	return nil
}
