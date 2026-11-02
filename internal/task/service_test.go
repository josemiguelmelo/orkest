package task

import (
	"context"
	"errors"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/workflow"
	"github.com/josemiguelmelo/orkest/internal/workspace"
)

type taskRepositoryStub struct {
	Repository
	task    Task
	created TaskInput
	updated bool
}

func (r *taskRepositoryStub) Task(context.Context, string, string, string) (Task, error) {
	return r.task, nil
}

func (r *taskRepositoryStub) CreateTask(_ context.Context, id, workspaceID string, input TaskInput) (Task, error) {
	r.created = input
	return Task{ID: id, WorkspaceID: workspaceID, Title: input.Title, Status: input.Status}, nil
}

func (r *taskRepositoryStub) UpdateTask(_ context.Context, _, workspaceID, id string, input TaskInput) (Task, error) {
	r.updated = true
	return Task{ID: id, WorkspaceID: workspaceID, Status: input.Status}, nil
}

type workspaceAccessStub struct{ role string }

func (w workspaceAccessStub) GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error) {
	return workspace.Workspace{Role: w.role}, nil
}

func TestCreateTaskDefaultsAndNormalizes(t *testing.T) {
	repository := &taskRepositoryStub{}
	service := testService(repository, "member")
	created, err := service.Create(context.Background(), "user", "org", "workspace", TaskInput{
		WorkflowID: "workspace", Title: "  Add task API  ", Labels: []string{" backend ", "backend", ""},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if created.Status != "todo" || repository.created.Status != "todo" {
		t.Fatalf("default status = %q, want todo", repository.created.Status)
	}
	if repository.created.Title != "Add task API" {
		t.Fatalf("normalized title = %q", repository.created.Title)
	}
	if len(repository.created.Labels) != 1 || repository.created.Labels[0] != "backend" {
		t.Fatalf("normalized labels = %#v", repository.created.Labels)
	}
}

func TestViewerCannotCreateTask(t *testing.T) {
	repository := &taskRepositoryStub{}
	service := testService(repository, "viewer")
	_, err := service.Create(context.Background(), "user", "org", "workspace", TaskInput{Title: "Task"})
	if !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatalf("create error = %v, want forbidden", err)
	}
}

func TestInvalidWorkflowStatusIsRejected(t *testing.T) {
	repository := &taskRepositoryStub{}
	service := testService(repository, "member")
	_, err := service.Create(context.Background(), "user", "org", "workspace", TaskInput{WorkflowID: "workspace", Title: "Task", Status: "blocked"})
	if !errors.Is(err, modelerrors.ErrInvalid) {
		t.Fatalf("create error = %v, want invalid", err)
	}
}

type definitionStub struct{ workflow.Repository }

func (definitionStub) Definitions(context.Context, string) ([]workflow.Definition, error) {
	return nil, nil
}

func (definitionStub) Definition(_ context.Context, _, workflowID string) (workflow.Definition, error) {
	return workflow.Definition{ID: workflowID, Name: "Workflow", Version: 1, Stages: []workflow.Stage{{ID: "todo"}, {ID: "done"}}}, nil
}

func (definitionStub) Create(context.Context, string, workflow.Definition) (workflow.Definition, error) {
	return workflow.Definition{}, nil
}

func (definitionStub) Save(context.Context, string, workflow.Definition) (workflow.Definition, error) {
	return workflow.Definition{}, nil
}

func (definitionStub) Delete(context.Context, string, string) error {
	return nil
}

func testService(repo *taskRepositoryStub, role string) *Service {
	access := workspaceAccessStub{role: role}
	return NewService(repo, access, workflow.NewService(definitionStub{}, access), nil)
}
