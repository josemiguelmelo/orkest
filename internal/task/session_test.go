package task

import (
	"context"
	"testing"
)

type sessionTaskRepository struct{ taskRepositoryStub }

func (r *sessionTaskRepository) UpdateTaskIfStatus(_ context.Context, _, _, _, _ string, input TaskInput) (Task, error) {
	r.updated = true
	result := r.task
	result.Status = input.Status
	return result, nil
}

type sessionDispatcher struct {
	received *Task
	accepted bool
}

func (d *sessionDispatcher) StartTask(_ context.Context, _, _ string, task Task) error {
	d.received = &task
	return nil
}

func (d *sessionDispatcher) TaskCommandAccepted(context.Context, string, string, string, string, string) (bool, error) {
	return d.accepted, nil
}

func TestSessionChoiceStartsReviewIteration(t *testing.T) {
	repo := &sessionTaskRepository{taskRepositoryStub{task: Task{ID: "task", WorkspaceID: "workspace", Title: "Task", Status: "in_review", AssigneeAgentID: "agent", SourceProvider: "manual"}}}
	dispatcher := &sessionDispatcher{}
	service := NewService(repo, workspaceAccessStub{role: "member"}, dispatcher)
	input := TaskInput{Title: "Task", Status: "in_progress", AssigneeAgentID: "agent", SessionMode: "new", PromptText: "revise this", CommandID: "turn"}
	if _, err := service.Update(context.Background(), "user", "org", "workspace", "task", input); err != nil {
		t.Fatal(err)
	}
	if dispatcher.received == nil || dispatcher.received.SessionMode != "new" || dispatcher.received.PromptText != "revise this" || dispatcher.received.CommandID != "turn" {
		t.Fatal("session choice or prompt lost")
	}
	dispatcher.received = nil
	dispatcher.accepted = true
	repo.updated = false
	if _, err := service.Update(context.Background(), "user", "org", "workspace", "task", input); err != nil {
		t.Fatal(err)
	}
	if dispatcher.received != nil || repo.updated {
		t.Fatal("duplicate start command changed workflow or created another run")
	}
}
