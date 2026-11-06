package workflowautomation_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/server/task"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	"github.com/josemiguelmelo/orkest/internal/server/workflowautomation"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type sessionDispatcher struct {
	received *taskmodel.StartCommand
	accepted bool
}

func (d *sessionDispatcher) StartTask(_ context.Context, _, _ string, value taskmodel.StartCommand) error {
	d.received = &value
	return nil
}

func (d *sessionDispatcher) TaskCommandAccepted(context.Context, string, string, string, string, string) (bool, error) {
	return d.accepted, nil
}

func TestWorkflowDeliveryPreservesSessionChoiceAndReconciles(t *testing.T) {
	repo := &taskRepositoryStub{task: taskmodel.Task{ID: "task", WorkspaceID: "workspace", Title: "Task", Status: "review", StageVersion: 2, AssigneeAgentID: "agent"}}
	dispatcher := &sessionDispatcher{}
	service := testService(repo, "member")
	command := workflowautomation.ExecutionCommand{ID: "turn", OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", ActorID: "user", StageVersion: 2, Options: workflowautomation.StartWorkOptions{SessionMode: "new", PromptText: "revise this"}}
	delivered, err := deliverCommand(context.Background(), service, dispatcher, command)
	if err != nil || !delivered || dispatcher.received == nil || dispatcher.received.PromptText != "revise this" || dispatcher.received.CommandID != "turn" || dispatcher.received.SessionMode != "new" {
		t.Fatal("session options were lost", err)
	}
	dispatcher.received = nil
	dispatcher.accepted = true
	if delivered, err = deliverCommand(context.Background(), service, dispatcher, command); err != nil || !delivered || dispatcher.received != nil {
		t.Fatal("accepted delivery was dispatched again", err)
	}
	dispatcher.accepted = false
	command.StageVersion = 1
	if delivered, err = deliverCommand(context.Background(), service, dispatcher, command); err != nil || delivered || dispatcher.received != nil {
		t.Fatal("stale stage entry was started", err)
	}
}

func TestWorkflowDeliveryLeavesUnspecifiedSessionChoiceForDispatcher(t *testing.T) {
	repo := &taskRepositoryStub{task: taskmodel.Task{ID: "task", WorkspaceID: "workspace", Title: "Task", Status: "review", StageVersion: 2, AssigneeAgentID: "agent"}}
	dispatcher := &sessionDispatcher{}
	service := testService(repo, "member")
	command := workflowautomation.ExecutionCommand{ID: "turn", OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", ActorID: "user", StageVersion: 2}
	delivered, err := deliverCommand(context.Background(), service, dispatcher, command)
	if err != nil || !delivered || dispatcher.received == nil || dispatcher.received.SessionMode != "" {
		t.Fatal("unspecified session choice was not preserved", err)
	}
}

func TestWorkflowDeliveryRechecksMutationPermission(t *testing.T) {
	repo := &taskRepositoryStub{task: taskmodel.Task{ID: "task", StageVersion: 1}}
	dispatcher := &sessionDispatcher{}
	service := testService(repo, "viewer")
	delivered, err := deliverCommand(context.Background(), service, dispatcher, workflowautomation.ExecutionCommand{StageVersion: 1})
	if err != nil || delivered || dispatcher.received != nil {
		t.Fatal("viewer automation started work", err)
	}
}

type eventRepositoryStub struct {
	workflowautomation.AutomationEventRepository
	called bool
	err    error
}

func (r *eventRepositoryStub) WithAutomationEvent(context.Context, func(workflowautomation.AutomationEventSnapshot) (workflowautomation.AutomationEventEffects, error)) (bool, error) {
	r.called = true
	return r.err == nil, r.err
}

type emptyExecutionCommandRepository struct{}

func (emptyExecutionCommandRepository) ClaimExecutionCommand(context.Context, string, time.Duration) (workflowautomation.ExecutionClaim, bool, error) {
	return workflowautomation.ExecutionClaim{}, false, nil
}

func (emptyExecutionCommandRepository) CompleteExecutionCommand(context.Context, workflowautomation.ExecutionClaim, func(bool) workflowautomation.ExecutionDeliveryEffects) error {
	return nil
}

func TestWorkflowWorkerProcessesEventsBeforeCommands(t *testing.T) {
	repo := &eventRepositoryStub{}
	service := workflowautomation.NewWorkflowAutomationService(nil, repo, nil, nil)
	execution := workflowautomation.NewWorkflowExecutionService(emptyExecutionCommandRepository{}, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	worker := workflowautomation.NewWorkflowAutomationWorker(service, execution, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if found, err := worker.ProcessOne(context.Background()); err != nil || !found || !repo.called {
		t.Fatal(found, err)
	}
	repo.err = errors.New("temporary database failure")
	if found, err := worker.ProcessOne(context.Background()); found || err == nil {
		t.Fatal("event failure acknowledged")
	}
}

// These adapters exercise delivery with the real task authorization service.
type taskRepositoryStub struct {
	task.TaskRepository
	task taskmodel.Task
}

func (r *taskRepositoryStub) Task(context.Context, string, string, string) (taskmodel.Task, error) {
	return r.task, nil
}

type workspaceAccessStub struct{ role string }

func (w workspaceAccessStub) GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error) {
	return workspacemodel.Workspace{Role: w.role}, nil
}

func testService(repo *taskRepositoryStub, role string) *task.TaskService {
	return task.NewTaskService(repo, nil, nil, nil, workspaceAccessStub{role: role})
}

type singleExecutionCommandRepository struct {
	command   workflowautomation.ExecutionCommand
	delivered bool
	failed    bool
	healthErr error
	effects   workflowautomation.ExecutionDeliveryEffects
}

func (r *singleExecutionCommandRepository) ClaimExecutionCommand(_ context.Context, token string, _ time.Duration) (workflowautomation.ExecutionClaim, bool, error) {
	return workflowautomation.ExecutionClaim{Command: r.command, Token: token, Attempt: 1}, true, nil
}

func (r *singleExecutionCommandRepository) CompleteExecutionCommand(_ context.Context, _ workflowautomation.ExecutionClaim, decide func(bool) workflowautomation.ExecutionDeliveryEffects) error {
	if r.healthErr != nil {
		return r.healthErr
	}
	r.effects = decide(r.failed)
	r.delivered = r.effects.CommandState == "delivered"
	return nil
}

func deliverCommand(ctx context.Context, tasks *task.TaskService, dispatcher *sessionDispatcher, command workflowautomation.ExecutionCommand) (bool, error) {
	repo := &singleExecutionCommandRepository{command: command}
	execution := workflowautomation.NewWorkflowExecutionService(repo, tasks, dispatcher, slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := execution.ProcessNextCommand(ctx)
	return repo.delivered, err
}

func TestExecutionServiceKeepsAcceptedFailuresVisible(t *testing.T) {
	repo := &singleExecutionCommandRepository{command: workflowautomation.ExecutionCommand{ID: "accepted", TaskID: "task"}, failed: true}
	dispatcher := &sessionDispatcher{accepted: true}
	service := workflowautomation.NewWorkflowExecutionService(repo, nil, dispatcher, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := service.ProcessNextCommand(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repo.effects.CommandState != "delivered" || repo.effects.WorkState != taskmodel.WorkStateFailed || dispatcher.received != nil {
		t.Fatalf("accepted failure was overwritten or redispatched: %+v", repo.effects)
	}
	repo.healthErr = errors.New("health lookup failed")
	if _, err := service.ProcessNextCommand(context.Background()); err == nil {
		t.Fatal("health failure was acknowledged")
	}
}
