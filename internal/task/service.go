package task

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/core/ids"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
)

type Service struct {
	repository Repository
	workspaces WorkspaceAccess
	dispatcher TaskDispatcher
}

type TaskDispatcher interface {
	StartTask(context.Context, string, string, Task) error
	TaskCommandAccepted(context.Context, string, string, string, string, string) (bool, error)
}

type DispatchError struct{ Cause error }

func (e *DispatchError) Error() string { return e.Cause.Error() }
func (e *DispatchError) Unwrap() error { return e.Cause }

func NewService(repository Repository, workspaces WorkspaceAccess, dispatchers ...TaskDispatcher) *Service {
	service := &Service{repository: repository, workspaces: workspaces}
	if len(dispatchers) > 0 {
		service.dispatcher = dispatchers[0]
	}
	return service
}

func (s *Service) List(ctx context.Context, userID, orgID, workspaceID string) ([]Task, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repository.Tasks(ctx, orgID, workspaceID)
}

func (s *Service) Get(ctx context.Context, userID, orgID, workspaceID, taskID string) (Task, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return Task{}, err
	}
	return s.repository.Task(ctx, orgID, workspaceID, taskID)
}

// AuthorizeExecution checks task visibility and whether the caller may interact with its run.
func (s *Service) AuthorizeExecution(ctx context.Context, userID, orgID, workspaceID, taskID string, mutate bool) error {
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

func (s *Service) Create(ctx context.Context, userID, orgID, workspaceID string, input TaskInput) (Task, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return Task{}, err
	}
	if err := normalizeTask(&input, true); err != nil {
		return Task{}, err
	}
	id, err := ids.NewUUID()
	if err != nil {
		return Task{}, fmt.Errorf("generate task ID: %w", err)
	}
	return s.repository.CreateTask(ctx, id, workspaceID, input)
}

func (s *Service) Update(ctx context.Context, userID, orgID, workspaceID, taskID string, input TaskInput) (Task, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return Task{}, err
	}
	if err := normalizeTask(&input, false); err != nil {
		return Task{}, err
	}
	current, err := s.repository.Task(ctx, orgID, workspaceID, taskID)
	if err != nil {
		return Task{}, err
	}
	if input.CommandID != "" && input.Status == "in_progress" {
		if s.dispatcher != nil {
			accepted, err := s.dispatcher.TaskCommandAccepted(ctx, orgID, workspaceID, taskID, input.CommandID, input.PromptText)
			if err != nil {
				return Task{}, err
			}
			if accepted {
				return current, nil
			}
		}
	}
	if current.SourceProvider != "manual" {
		// Source-managed fields can change during a sync while the user is editing.
		// Keep the latest source values and apply only local workflow fields.
		input.Title = current.Title
		input.Description = current.Description
		input.Labels = current.Labels
	}
	if current.Status == "done" && input.Status != "done" {
		return Task{}, modelerrors.ErrConflict
	}
	starting := (current.Status != "in_progress" || input.StartWork) && input.Status == "in_progress"
	if starting && strings.TrimSpace(input.AssigneeAgentID) == "" {
		return Task{}, modelerrors.ErrConflict
	}
	if starting && s.dispatcher == nil {
		return Task{}, errors.New("task execution dispatcher is not configured")
	}
	var updated Task
	if starting {
		updated, err = s.repository.UpdateTaskIfStatus(ctx, orgID, workspaceID, taskID, current.Status, input)
	} else {
		updated, err = s.repository.UpdateTask(ctx, orgID, workspaceID, taskID, input)
	}
	if err != nil {
		return Task{}, err
	}
	if starting && s.dispatcher != nil {
		updated.SessionMode, updated.PromptText, updated.CommandID = input.SessionMode, input.PromptText, input.CommandID
		if err = s.dispatcher.StartTask(ctx, userID, orgID, updated); err != nil {
			return updated, &DispatchError{Cause: fmt.Errorf("task moved to in_progress but agent dispatch failed: %w", err)}
		}
	}
	return updated, nil
}

func (s *Service) Comments(ctx context.Context, userID, orgID, workspaceID, taskID string) ([]Comment, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	if _, err := s.repository.Task(ctx, orgID, workspaceID, taskID); err != nil {
		return nil, err
	}
	return s.repository.Comments(ctx, orgID, workspaceID, taskID)
}

func (s *Service) AddComment(ctx context.Context, userID, orgID, workspaceID, taskID, body string) (Comment, error) {
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

func (s *Service) Components(ctx context.Context, userID, orgID, workspaceID string) ([]Component, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repository.Components(ctx, orgID, workspaceID)
}

func (s *Service) CreateComponent(ctx context.Context, userID, orgID, workspaceID string, input ComponentInput) (Component, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return Component{}, err
	}
	if err := normalizeComponent(&input); err != nil {
		return Component{}, err
	}
	id, err := ids.NewUUID()
	if err != nil {
		return Component{}, fmt.Errorf("generate component ID: %w", err)
	}
	return s.repository.CreateComponent(ctx, id, workspaceID, input)
}

func (s *Service) UpdateComponent(ctx context.Context, userID, orgID, workspaceID, componentID string, input ComponentInput) (Component, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return Component{}, err
	}
	if err := normalizeComponent(&input); err != nil {
		return Component{}, err
	}
	return s.repository.UpdateComponent(ctx, orgID, workspaceID, componentID, input)
}

func (s *Service) authorize(ctx context.Context, userID, orgID, workspaceID string) error {
	w, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return err
	}
	if w.Role == "viewer" {
		return modelerrors.ErrForbidden
	}
	return nil
}

func normalizeTask(input *TaskInput, creating bool) error {
	if input.SessionMode != "" && input.SessionMode != "continue" && input.SessionMode != "new" {
		return modelerrors.ErrInvalid
	}
	if len(input.PromptText) > 256*1024 || len(input.CommandID) > 200 || (input.CommandID != "" && strings.TrimSpace(input.CommandID) == "") {
		return modelerrors.ErrInvalid
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	if input.Title == "" || len(input.Title) > 500 || len(input.Description) > 100000 {
		return modelerrors.ErrInvalid
	}
	if creating && input.Status == "" {
		input.Status = "todo"
	}
	switch input.Status {
	case "todo", "in_progress", "in_review", "done":
	default:
		return modelerrors.ErrInvalid
	}
	input.AssigneeAgentID = strings.TrimSpace(input.AssigneeAgentID)
	input.Labels = normalizeStrings(input.Labels)
	input.ComponentIDs = normalizeStrings(input.ComponentIDs)
	return nil
}

func normalizeComponent(input *ComponentInput) error {
	input.Identifier = strings.TrimSpace(input.Identifier)
	input.Name = strings.TrimSpace(input.Name)
	input.RepositoryID = strings.TrimSpace(input.RepositoryID)
	input.Description = strings.TrimSpace(input.Description)
	if input.Identifier == "" || len(input.Identifier) > 100 || input.Name == "" || len(input.Name) > 200 || input.RepositoryID == "" || len(input.Description) > 2000 {
		return modelerrors.ErrInvalid
	}
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
