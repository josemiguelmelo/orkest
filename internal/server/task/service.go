package task

import (
	"context"
	"fmt"
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
)

type TaskService struct {
	repository TaskRepository
	workspaces WorkspaceAccess
	creations  TaskCreationRepository
	updates    TaskUpdateRepository
	workflow   TaskWorkflowPlanner
}

type DispatchError struct{ Cause error }

func (e *DispatchError) Error() string { return e.Cause.Error() }
func (e *DispatchError) Unwrap() error { return e.Cause }

func NewTaskService(repository TaskRepository, creations TaskCreationRepository, updates TaskUpdateRepository, workflow TaskWorkflowPlanner, workspaces WorkspaceAccess) *TaskService {
	return &TaskService{repository: repository, creations: creations, updates: updates, workflow: workflow, workspaces: workspaces}
}

func (s *TaskService) List(ctx context.Context, userID, orgID, workspaceID string) ([]Task, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repository.Tasks(ctx, orgID, workspaceID)
}

func (s *TaskService) Get(ctx context.Context, userID, orgID, workspaceID, taskID string) (Task, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return Task{}, err
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

func (s *TaskService) Comments(ctx context.Context, userID, orgID, workspaceID, taskID string) ([]Comment, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	if _, err := s.repository.Task(ctx, orgID, workspaceID, taskID); err != nil {
		return nil, err
	}
	return s.repository.Comments(ctx, orgID, workspaceID, taskID)
}

func (s *TaskService) AddComment(ctx context.Context, userID, orgID, workspaceID, taskID, body string) (Comment, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return Comment{}, err
	}
	if _, err := s.repository.Task(ctx, orgID, workspaceID, taskID); err != nil {
		return Comment{}, err
	}
	body = strings.TrimSpace(body)
	if body == "" || len(body) > 20000 {
		return Comment{}, modelerrors.ErrInvalid
	}
	id, err := ids.NewUUID()
	if err != nil {
		return Comment{}, fmt.Errorf("generate comment ID: %w", err)
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

func NormalizeTaskContent(input *TaskInput) error {
	if input.StageVersion < 0 || (input.StartWork && input.CompleteStage) || len(input.Status) > 100 {
		return modelerrors.ErrInvalid
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	if input.Title == "" || len(input.Title) > 500 || len(input.Description) > 100000 {
		return modelerrors.ErrInvalid
	}
	input.AssigneeAgentID = strings.TrimSpace(input.AssigneeAgentID)
	input.Labels = normalizeStrings(input.Labels)
	input.RepositoryIDs = normalizeStrings(input.RepositoryIDs)
	return nil
}

func normalizeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
