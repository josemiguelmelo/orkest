package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	"github.com/josemiguelmelo/orkest/internal/server/task"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	trigger "github.com/josemiguelmelo/orkest/internal/server/tasktriggerintegration"
	"github.com/josemiguelmelo/orkest/internal/server/workflowautomation"
)

func testTaskTriggerIntegrations(t *testing.T, db *PostgresDatabase, org, workspace, actor string) {
	t.Helper()
	ctx := context.Background()
	newID := func() string {
		v, err := ids.NewUUID()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	tasks := NewTaskPostgresRepository(db)
	taskID := newID()
	if _, err := createWorkflowTask(ctx, tasks, taskID, workspace, taskmodel.TaskInput{WorkflowID: workspace, Title: "External trigger", Status: "todo"}); err != nil {
		t.Fatal(err)
	}
	repo := NewTaskTriggerIntegrationPostgresRepository(db)
	integration, err := repo.Save(ctx, trigger.Integration{ID: newID(), WorkspaceID: workspace, OrganizationID: org, Name: "Review hooks", Provider: "github", Config: json.RawMessage(`{"repositoryId":"123"}`), Enabled: true, ConfiguredBy: actor, EncryptedSecret: "encrypted", Rules: []trigger.Rule{{ID: "review", Event: "pull_request.review_submitted", WorkflowID: workspace, SourceStageID: "todo", TargetStageID: "in_review", Action: "move_task"}}}, true)
	if err != nil {
		t.Fatal(err)
	}
	// Cross-workspace and unselected repositories cannot establish links.
	if err = repo.Link(ctx, integration, trigger.ResourceLink{TaskID: taskID, RepositoryID: "123", ResourceID: "4"}); err == nil {
		t.Fatal("linked unselected repository")
	}

	repositoryID := newID()
	if _, err = db.pool.Exec(ctx, `INSERT INTO code_repositories(id,workspace_id,provider_name,url,provider_repository_id) VALUES($1,$2,'github','https://github.com/owner/trigger-test','123');`, repositoryID, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err = db.pool.Exec(ctx, `INSERT INTO task_repositories(workspace_id,task_id,repository_id) VALUES($1,$2,$3)`, workspace, taskID, repositoryID); err != nil {
		t.Fatal(err)
	}
	if err = repo.Link(ctx, integration, trigger.ResourceLink{TaskID: taskID, RepositoryID: "123", ResourceID: "4"}); err != nil {
		t.Fatal(err)
	}
	event := trigger.Event{Key: "review:1", Type: "pull_request.review_submitted", RepositoryID: "123", ResourceID: "4", OccurredAt: time.Now().Add(time.Second)}
	if err = repo.Ingest(ctx, integration, "delivery-1", []trigger.Event{event}); err != nil {
		t.Fatal(err)
	}
	if err = repo.Ingest(ctx, integration, "delivery-2", []trigger.Event{event}); err != nil {
		t.Fatal(err)
	}
	deliveries, err := repo.Deliveries(ctx, integration.ID)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("duplicate deliveries: %d %v", len(deliveries), err)
	}
	sentinel := errors.New("rollback after planning")
	// Force acknowledgement failure after planning a real movement: neither
	// the movement nor its outbox event may survive the transaction rollback.
	decide := func(ctx context.Context, s trigger.ProcessingSnapshot) (trigger.ProcessingResult, error) {
		result, e := workflowautomation.PlanTaskTransition(workflowautomation.TransitionSnapshot{Workflow: s.Task.Workflow, Task: s.Task.Task, ActorID: actor}, workflowautomation.TransitionCommand{Input: workflowautomation.TransitionRequest{Action: "move", StageID: "in_review", StageVersion: s.StageVersion, CommandID: s.Delivery.ID}})
		if e != nil {
			return trigger.ProcessingResult{}, e
		}
		return trigger.ProcessingResult{Status: "invalid-status", Effects: &task.TaskWorkflowEffects{Command: task.WorkflowCommand(result.Input), Persistence: result.Persistence, StageID: result.Plan.StageID, AssigneeAgentID: result.Plan.AssigneeAgentID, Moved: result.Moved, StageVersion: result.StageVersion}}, nil
	}
	if _, err = repo.ProcessNext(ctx, decide); err == nil {
		t.Fatal("expected status constraint rollback")
	}
	loaded, err := tasks.Task(ctx, org, workspace, taskID)
	if err != nil || loaded.Status != "todo" || loaded.StageVersion != 1 {
		t.Fatalf("movement survived rollback: %+v %v", loaded, err)
	}
	deliveries, err = repo.Deliveries(ctx, integration.ID)
	if err != nil || deliveries[0].Status != "pending" {
		t.Fatalf("ack survived rollback: %+v %v", deliveries, err)
	}
	if _, err = repo.ProcessNext(ctx, func(context.Context, trigger.ProcessingSnapshot) (trigger.ProcessingResult, error) {
		return trigger.ProcessingResult{}, sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	results := make(chan bool, 2)
	failures := make(chan error, 2)
	for range 2 {
		workers.Go(func() {
			found, e := repo.ProcessNext(ctx, func(ctx context.Context, s trigger.ProcessingSnapshot) (trigger.ProcessingResult, error) {
				result, e := decide(ctx, s)
				result.Status = "processed"
				return result, e
			})
			results <- found
			failures <- e
		})
	}
	workers.Wait()
	close(results)
	close(failures)
	processed := 0
	for found := range results {
		if found {
			processed++
		}
	}
	for e := range failures {
		if e != nil {
			t.Fatal(e)
		}
	}
	if processed != 1 {
		t.Fatalf("concurrent workers processed %d deliveries", processed)
	}
	loaded, err = tasks.Task(ctx, org, workspace, taskID)
	if err != nil || loaded.Status != "in_review" || loaded.StageVersion != 2 {
		t.Fatalf("movement failed: %+v %v", loaded, err)
	}
	deliveries, err = repo.Deliveries(ctx, integration.ID)
	if err != nil || deliveries[0].Status != "processed" {
		t.Fatalf("not acknowledged: %+v %v", deliveries, err)
	}
	if err = repo.Ingest(ctx, integration, "delivery-3", []trigger.Event{event}); err != nil {
		t.Fatal(err)
	}
	if found, e := repo.ProcessNext(ctx, decide); e != nil || found {
		t.Fatalf("replay processed: %v %v", found, e)
	}
}
