package orchestration

import (
	"context"
	"errors"
	"testing"

	model "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

type editorGuardFake struct {
	calls int
	err   error
}

func (g *editorGuardFake) BeforeExecution(context.Context, string, string, string) (func(), error) {
	g.calls++
	if g.err != nil {
		return nil, g.err
	}
	return func() {}, nil
}

type editorRepoFake struct {
	launchRepoFake
	items   []model.Launch
	pending []PromptDelivery
}

func (r *editorRepoFake) ListTaskLaunches(context.Context, string, string, string) ([]model.Launch, error) {
	return r.items, nil
}

func (r *editorRepoFake) PendingPrompts(context.Context, string) ([]PromptDelivery, error) {
	return r.pending, nil
}

type editorProcessFake struct {
	sessionProcessController
	running bool
	err     error
}

func (c *editorProcessFake) AgentctlRunning(context.Context, string) (bool, error) {
	return c.running, c.err
}

func (c *editorProcessFake) AgentctlRunningByRunID(context.Context, string) (bool, error) {
	return c.running, c.err
}

func TestEditorIdleRequiresTerminalStateAndConfirmedProcessAbsence(t *testing.T) {
	for _, tc := range []struct {
		name          string
		status        model.LaunchStatus
		running       bool
		inspectionErr error
		idle          bool
	}{
		{name: "pending start", status: model.LaunchStarting},
		{name: "live ready conversation", status: model.LaunchReady},
		{name: "cleanup pending", status: model.LaunchCompletedPendingClose},
		{name: "terminal but process alive", status: model.LaunchCompleted, running: true},
		{name: "inspection uncertain", status: model.LaunchFailed, inspectionErr: errors.New("offline")},
		{name: "completed and stopped", status: model.LaunchCompleted, idle: true},
		{name: "failed and stopped", status: model.LaunchFailed, idle: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &editorRepoFake{items: []model.Launch{{RunID: "run", ProcessID: "process", Status: tc.status}}}
			completion, err := NewLaunchCompletionService(context.Background(), repo, &editorProcessFake{running: tc.running, err: tc.inspectionErr})
			if err != nil {
				t.Fatal(err)
			}
			defer completion.Close()
			service := &LaunchService{repo: repo, completion: completion}
			idle, _ := service.EditorIdle(context.Background(), "org", "workspace", "task")
			if idle != tc.idle {
				t.Fatalf("idle=%v want %v", idle, tc.idle)
			}
		})
	}
}

func TestEditorHandoffFailureBlocksEveryDeliveryBoundary(t *testing.T) {
	ctx := context.Background()
	guard := &editorGuardFake{err: errors.New("editor termination uncertain")}
	repo := &editorRepoFake{items: []model.Launch{{RunID: "run", OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", Status: model.LaunchReady, Endpoint: "http://worker"}}, pending: []PromptDelivery{{CommandID: "command", Text: "work"}}}
	client := &agentctlFake{}
	service := &LaunchService{repo: repo, client: client, executor: launcherFake{}, editorGuard: guard}
	if _, err := service.Start(ctx, validConfig()); err == nil {
		t.Fatal("start bypassed editor stop failure")
	}
	if err := service.SendTaskPrompt(ctx, "org", "workspace", "task", "run", "command", "work"); err == nil {
		t.Fatal("follow-up bypassed stop failure")
	}
	if err := service.TaskQueue(ctx, "org", "workspace", "task", "run", false); err == nil {
		t.Fatal("queue resume bypassed stop failure")
	}
	connection := &ConnectionService{repo: repo, client: client, editorGuard: guard}
	if err := connection.reconcilePrompts(ctx, repo.items[0]); err == nil {
		t.Fatal("outbox replay bypassed stop failure")
	}
	if guard.calls != 4 {
		t.Fatalf("guard calls=%d want 4", guard.calls)
	}
	if len(client.calls) != 0 {
		t.Fatalf("execution occurred: %v", client.calls)
	}
}
