package task

import (
	"context"
	"errors"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/workflow"
	"github.com/josemiguelmelo/orkest/internal/server/workspace"
)

type creationRepositoryStub struct{ created TaskInput }

func (r *creationRepositoryStub) WithTaskCreation(_ context.Context, id, workspaceID string, input TaskInput, decide func(TaskCreationSnapshot, TaskInput) (TaskCreationEffects, error)) (Task, error) {
	effects, err := decide(TaskCreationSnapshot{Workflow: workflow.DefaultDefinition("Board")}, input)
	if err != nil {
		return Task{}, err
	}
	r.created = effects.Input
	return Task{ID: id, WorkspaceID: workspaceID, Title: r.created.Title, Status: r.created.Status}, nil
}

type creationAccessStub struct{ role string }

func (a creationAccessStub) GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error) {
	return workspace.Workspace{Role: a.role}, nil
}

func creationTestService(repo *creationRepositoryStub, role string) *TaskService {
	return NewTaskService(nil, repo, nil, nil, creationAccessStub{role: role})
}

func TestCreateTaskDefaultsAndNormalizes(t *testing.T) {
	repository := &creationRepositoryStub{}
	service := creationTestService(repository, "member")
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
	repository := &creationRepositoryStub{}
	service := creationTestService(repository, "viewer")
	_, err := service.Create(context.Background(), "user", "org", "workspace", TaskInput{Title: "Task"})
	if !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatalf("create error = %v, want forbidden", err)
	}
}

func TestInvalidWorkflowStatusIsRejected(t *testing.T) {
	repository := &creationRepositoryStub{}
	service := creationTestService(repository, "member")
	_, err := service.Create(context.Background(), "user", "org", "workspace", TaskInput{WorkflowID: "workspace", Title: "Task", Status: "blocked"})
	if !errors.Is(err, modelerrors.ErrInvalid) {
		t.Fatalf("create error = %v, want invalid", err)
	}
}
