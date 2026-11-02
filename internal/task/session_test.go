package task

import (
	"context"
	"io"
	"log/slog"
	"testing"
)

type sessionDispatcher struct {
	received *Task
	accepted bool
}

func (d *sessionDispatcher) StartTask(_ context.Context, _, _ string, value Task) error {
	d.received = &value
	return nil
}

func (d *sessionDispatcher) TaskCommandAccepted(context.Context, string, string, string, string, string) (bool, error) {
	return d.accepted, nil
}

func TestWorkflowDeliveryPreservesSessionChoiceAndReconciles(t *testing.T) {
	repo := &taskRepositoryStub{task: Task{ID: "task", WorkspaceID: "workspace", Title: "Task", Status: "review", StageVersion: 2, AssigneeAgentID: "agent"}}
	dispatcher := &sessionDispatcher{}
	worker := NewWorkflowWorker(nil, testService(repo, "member"), dispatcher, slog.New(slog.NewTextHandler(io.Discard, nil)))
	command := WorkflowCommand{ID: "turn", OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", ActorID: "user", StageVersion: 2, Options: TransitionInput{SessionMode: "new", PromptText: "revise this"}}
	delivered, err := worker.deliver(context.Background(), command)
	if err != nil || !delivered || dispatcher.received == nil || dispatcher.received.PromptText != "revise this" || dispatcher.received.CommandID != "turn" || dispatcher.received.SessionMode != "new" {
		t.Fatal("session options were lost", err)
	}
	dispatcher.received = nil
	dispatcher.accepted = true
	if delivered, err = worker.deliver(context.Background(), command); err != nil || !delivered || dispatcher.received != nil {
		t.Fatal("accepted delivery was dispatched again", err)
	}
	dispatcher.accepted = false
	command.StageVersion = 1
	if delivered, err = worker.deliver(context.Background(), command); err != nil || delivered || dispatcher.received != nil {
		t.Fatal("stale stage entry was started", err)
	}
}

func TestWorkflowDeliveryRechecksMutationPermission(t *testing.T) {
	repo := &taskRepositoryStub{task: Task{ID: "task", StageVersion: 1}}
	dispatcher := &sessionDispatcher{}
	worker := NewWorkflowWorker(nil, testService(repo, "viewer"), dispatcher, slog.New(slog.NewTextHandler(io.Discard, nil)))
	delivered, err := worker.deliver(context.Background(), WorkflowCommand{StageVersion: 1})
	if err != nil || delivered || dispatcher.received != nil {
		t.Fatal("viewer automation started work", err)
	}
}
