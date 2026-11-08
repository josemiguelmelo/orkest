package tasktriggerintegration_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	githubtrigger "github.com/josemiguelmelo/orkest/internal/adapters/integrations/tasktriggerintegration/providers/github"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/task"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	trigger "github.com/josemiguelmelo/orkest/internal/server/tasktriggerintegration"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type access struct {
	role string
	err  error
}

func (a access) GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error) {
	return workspacemodel.Workspace{Role: a.role}, a.err
}

type repository struct {
	trigger.TaskTriggerIntegrationRepository
	snapshot trigger.ProcessingSnapshot
	result   trigger.ProcessingResult
}

func (r *repository) ProcessNext(ctx context.Context, decide func(context.Context, trigger.ProcessingSnapshot) (trigger.ProcessingResult, error)) (bool, error) {
	var err error
	r.result, err = decide(ctx, r.snapshot)
	return true, err
}

func TestProcessingPolicy(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mutate     func(*trigger.ProcessingSnapshot)
		role, want string
	}{
		{"moves linked idle task", func(*trigger.ProcessingSnapshot) {}, "admin", "processed"},
		{"revoked configuration access", func(*trigger.ProcessingSnapshot) {}, "viewer", "blocked"},
		{"disabled integration", func(s *trigger.ProcessingSnapshot) { s.Integration.Enabled = false }, "admin", "ignored"},
		{"stale stage", func(s *trigger.ProcessingSnapshot) { s.StageVersion = 1 }, "admin", "ignored"},
		{"active execution", func(s *trigger.ProcessingSnapshot) { s.Task.ActiveLaunch = true }, "admin", "blocked"},
		{"waiting execution", func(s *trigger.ProcessingSnapshot) { s.Task.Task.StageWorkState = taskmodel.WorkStateRunning }, "admin", "blocked"},
		{"wrong source stage", func(s *trigger.ProcessingSnapshot) { s.Integration.Rules[0].SourceStageID = "todo" }, "admin", "ignored"},
		{"closed task", func(s *trigger.ProcessingSnapshot) { s.Task.Task.Closed = true }, "admin", "ignored"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			definition := workflowmodel.DefaultDefinition("Default")
			definition.ID = "workflow"
			r := &repository{snapshot: trigger.ProcessingSnapshot{Integration: trigger.Integration{ID: "integration", Provider: "github", Config: json.RawMessage(`{"repositoryId":"123"}`), Enabled: true, ConfiguredBy: "admin", Rules: []trigger.Rule{{ID: "rule", Event: githubtrigger.ReviewSubmitted, WorkflowID: "workflow", SourceStageID: "in_review", TargetStageID: "in_progress"}}}, Delivery: trigger.Delivery{ID: "event-id", Event: trigger.Event{Type: githubtrigger.ReviewSubmitted, RepositoryID: "123", ResourceID: "4", ReferenceURL: "https://github.com/owner/repo/pull/4"}}, StageVersion: 2, Task: task.TaskWorkflowSnapshot{Workflow: definition, Task: taskmodel.Task{ID: "task", WorkflowID: "workflow", Status: "in_review", StageVersion: 2, StageWorkState: taskmodel.WorkStateIdle}}}}
			r.snapshot.ActorRole = tc.role
			tc.mutate(&r.snapshot)
			service, err := trigger.NewTaskTriggerIntegrationService(r, access{role: tc.role}, nil, githubtrigger.NewGitHubProvider())
			if err != nil {
				t.Fatal(err)
			}
			if _, err = service.ProcessNext(context.Background()); err != nil {
				t.Fatal(err)
			}
			if r.result.Status != tc.want {
				t.Fatalf("got %s want %s", r.result.Status, tc.want)
			}
			if tc.want == "processed" {
				e := r.result.Effects
				if e == nil || !e.Moved || e.StageVersion != 3 || len(e.Persistence.Events) != 1 || e.Persistence.Events[0].PromptText == "" || e.Start {
					t.Fatalf("unexpected effects: %+v", e)
				}
			} else if r.result.Effects != nil {
				t.Fatal("unexpected effects")
			}
		})
	}
}

func TestReviewStartRuleStartsAssignedAgentInSameStage(t *testing.T) {
	definition := workflowmodel.DefaultDefinition("Default")
	definition.ID = "workflow"
	prompt := "Read review comments and reply with the fix."
	r := &repository{snapshot: trigger.ProcessingSnapshot{
		Integration:  trigger.Integration{ID: "integration", Provider: "github", Config: json.RawMessage(`{"repositoryId":"123"}`), Enabled: true, ConfiguredBy: "admin", Rules: []trigger.Rule{{ID: "rule", Event: githubtrigger.ReviewSubmitted, WorkflowID: "workflow", SourceStageID: "in_review", TargetStageID: "in_review", Action: "start_work", PromptText: prompt}}},
		Delivery:     trigger.Delivery{ID: "event-id", Event: trigger.Event{Type: githubtrigger.ReviewSubmitted, RepositoryID: "123", ResourceID: "4", ReferenceURL: "https://github.com/owner/repo/pull/4"}},
		StageVersion: 2,
		ActorRole:    "admin",
		Task:         task.TaskWorkflowSnapshot{Workflow: definition, Task: taskmodel.Task{ID: "task", WorkflowID: "workflow", Status: "in_review", StageVersion: 2, StageWorkState: taskmodel.WorkStateIdle, AssigneeAgentID: "agent"}},
	}}
	service, err := trigger.NewTaskTriggerIntegrationService(r, access{role: "admin"}, nil, githubtrigger.NewGitHubProvider())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.ProcessNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	effects := r.result.Effects
	if r.result.Status != "processed" || effects == nil || !effects.Start || effects.Moved || effects.StageVersion != 2 || effects.Persistence.Execution == nil {
		t.Fatalf("unexpected same-stage start effects: status=%s effects=%+v", r.result.Status, effects)
	}
	options := effects.Persistence.Execution.Options
	if options.SessionMode != "continue" || !strings.Contains(options.PromptText, prompt) || !strings.Contains(options.PromptText, "https://github.com/owner/repo/pull/4") || !strings.Contains(options.PromptText, "inline review comments") {
		t.Fatalf("unexpected execution options: %+v", options)
	}
}

func TestViewersCannotConfigure(t *testing.T) {
	service, err := trigger.NewTaskTriggerIntegrationService(nil, access{role: "viewer"}, nil, githubtrigger.NewGitHubProvider())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Save(context.Background(), "user", "org", "workspace", "", trigger.Input{}); !errors.Is(err, problem.ErrForbidden) {
		t.Fatal(err)
	}
}

func TestDuplicateProvidersRejected(t *testing.T) {
	if _, err := trigger.NewTaskTriggerIntegrationService(nil, nil, nil, githubtrigger.NewGitHubProvider(), githubtrigger.NewGitHubProvider()); err == nil {
		t.Fatal("expected duplicate provider error")
	}
}
