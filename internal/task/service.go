package task

import (
	"context"
	"fmt"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/core/ids"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/workflow"
)

type WorkflowReader interface {
	Get(context.Context, string, string, string, string) (workflow.Definition, error)
}

type Service struct {
	repository  Repository
	workspaces  WorkspaceAccess
	workflows   WorkflowReader
	transitions TransitionRepository
}

type TaskDispatcher interface {
	StartTask(context.Context, string, string, Task) error
	TaskCommandAccepted(context.Context, string, string, string, string, string) (bool, error)
}

type DispatchError struct{ Cause error }

func (e *DispatchError) Error() string { return e.Cause.Error() }
func (e *DispatchError) Unwrap() error { return e.Cause }

func NewService(repository Repository, workspaces WorkspaceAccess, workflows WorkflowReader, transitions TransitionRepository) *Service {
	return &Service{repository: repository, workspaces: workspaces, workflows: workflows, transitions: transitions}
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
	if input.WorkflowID == "" {
		return Task{}, modelerrors.ErrInvalid
	}
	d, e := s.workflows.Get(ctx, userID, orgID, workspaceID, input.WorkflowID)
	if e != nil {
		return Task{}, e
	}
	if input.Status != "" && input.Status != d.StartStageID() {
		return Task{}, modelerrors.ErrInvalid
	}
	input.Status = d.StartStageID()
	input.ActorID = userID
	if err := normalizeTaskContent(&input); err != nil {
		return Task{}, err
	}
	id, err := ids.NewUUID()
	if err != nil {
		return Task{}, fmt.Errorf("generate task ID: %w", err)
	}
	return s.repository.CreateTask(ctx, id, workspaceID, input)
}

func (s *Service) Update(ctx context.Context, userID, orgID, workspaceID, taskID string, input TaskInput) (Task, error) {
	return s.updateWorkflowTask(ctx, userID, orgID, workspaceID, taskID, input)
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

func normalizeTaskContent(input *TaskInput) error {
	if input.StageVersion < 0 || (input.StartWork && input.CompleteStage) || len(input.Status) > 100 {
		return modelerrors.ErrInvalid
	}
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
