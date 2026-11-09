package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	"github.com/josemiguelmelo/orkest/internal/server/pullrequestlink"
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
	integration, err := saveTestTriggerConfiguration(ctx, repo, trigger.Integration{ID: newID(), WorkspaceID: workspace, OrganizationID: org, Name: "Review hooks", Provider: "github", Config: json.RawMessage(`{"repositoryIds":["123"]}`), Enabled: true, ConfiguredBy: actor, EncryptedSecret: "encrypted", Rules: []trigger.Rule{{ID: "review", Event: "pull_request.review_submitted", WorkflowID: workspace, SourceStageID: "todo", TargetStageID: "in_review", Action: "move_task"}}}, true)
	if err != nil {
		t.Fatal(err)
	}
	stale := integration
	stale.Version--
	if err := ingestTestTriggerEvents(ctx, repo, stale, "stale-config", []trigger.Event{{Key: "stale-config"}}); !errors.Is(err, problem.ErrConflict) {
		t.Fatalf("stale configuration accepted: %v", err)
	}
	if deliveries, err := repo.Deliveries(ctx, integration.ID); err != nil || len(deliveries) != 0 {
		t.Fatalf("stale ingestion left deliveries: %v %v", deliveries, err)
	}
	// Cross-workspace and unselected repositories cannot establish links.
	if err = seedAutomaticPullRequestLink(ctx, repo, integration.ID, trigger.ResourceLink{TaskID: taskID, RepositoryID: "123", ResourceID: "4"}); err == nil {
		t.Fatal("linked unselected repository")
	}

	repositoryID := newID()
	if _, err = db.pool.Exec(ctx, `INSERT INTO code_repositories(id,workspace_id,provider_name,url,provider_repository_id) VALUES($1,$2,'github','https://github.com/owner/trigger-test','123');`, repositoryID, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err = db.pool.Exec(ctx, `INSERT INTO task_repositories(workspace_id,task_id,repository_id) VALUES($1,$2,$3)`, workspace, taskID, repositoryID); err != nil {
		t.Fatal(err)
	}
	if err = seedAutomaticPullRequestLink(ctx, repo, integration.ID, trigger.ResourceLink{TaskID: taskID, RepositoryID: "123", ResourceID: "4"}); err != nil {
		t.Fatal(err)
	}
	event := trigger.Event{Key: "review:1", Type: "pull_request.review_submitted", RepositoryID: "123", ResourceID: "4", OccurredAt: time.Now().Add(time.Second)}
	if err = ingestTestTriggerEvents(ctx, repo, integration, "delivery-1", []trigger.Event{event}); err != nil {
		t.Fatal(err)
	}
	if err = ingestTestTriggerEvents(ctx, repo, integration, "delivery-2", []trigger.Event{event}); err != nil {
		t.Fatal(err)
	}
	// An existing link must not let ingestion bypass association/stage guards.
	if _, found, err := repo.NextProcessing(ctx); err != nil || found {
		t.Fatalf("ingestion bypassed resolution: %v %v", found, err)
	}
	// Every event now passes through the association service before processing.
	resolver := pullrequestlink.NewPullRequestLinkService(NewPullRequestLinkRepository(db))
	if err := resolver.ResolveEvent(ctx, integration.ID, event.Key); err != nil {
		t.Fatal(err)
	}
	deliveries, err := repo.Deliveries(ctx, integration.ID)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("duplicate deliveries: %d %v", len(deliveries), err)
	}
	// Force acknowledgement failure after planning a real movement: neither
	// the movement nor its outbox event may survive the transaction rollback.
	snapshot, found, err := repo.NextProcessing(ctx)
	if err != nil || !found {
		t.Fatalf("snapshot: %v %v", found, err)
	}
	again, found, err := repo.NextProcessing(ctx)
	if err != nil || !found || !reflect.DeepEqual(snapshot, again) {
		t.Fatal("snapshot read changed or consumed delivery", err)
	}
	result := testTriggerDecision(t, snapshot, actor, "invalid-status")
	if _, err = db.pool.Exec(ctx, `UPDATE task_trigger_deliveries SET diagnostic='changed while planning' WHERE id=$1`, snapshot.Delivery.ID); err != nil {
		t.Fatal(err)
	}
	if committed, err := repo.CommitProcessing(ctx, snapshot, result); err != nil || committed {
		t.Fatalf("stale snapshot applied: %v %v", committed, err)
	}
	snapshot, found, err = repo.NextProcessing(ctx)
	if err != nil || !found {
		t.Fatal(err)
	}
	result = testTriggerDecision(t, snapshot, actor, "invalid-status")
	if _, err = repo.CommitProcessing(ctx, snapshot, result); err == nil {
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
	var workers sync.WaitGroup
	results := make(chan bool, 2)
	failures := make(chan error, 2)
	for range 2 {
		workers.Go(func() {
			snapshot, found, e := repo.NextProcessing(ctx)
			if e == nil && found {
				result := testTriggerDecision(t, snapshot, actor, "processed")
				found, e = repo.CommitProcessing(ctx, snapshot, result)
			}
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
	if err = ingestTestTriggerEvents(ctx, repo, integration, "delivery-3", []trigger.Event{event}); err != nil {
		t.Fatal(err)
	}
	if _, found, e := repo.NextProcessing(ctx); e != nil || found {
		t.Fatalf("replay processed: %v %v", found, e)
	}
}

func testTriggerDecision(t *testing.T, s trigger.ProcessingSnapshot, actor, status string) trigger.ProcessingResult {
	t.Helper()
	result, err := workflowautomation.PlanTaskTransition(workflowautomation.TransitionSnapshot{Workflow: s.Task.Workflow, Task: s.Task.Task, ActorID: actor}, workflowautomation.TransitionCommand{Input: workflowautomation.TransitionRequest{Action: "move", StageID: "in_review", StageVersion: s.StageVersion, CommandID: s.Delivery.ID}})
	if err != nil {
		t.Fatal(err)
	}
	effects := result.TaskWorkflowEffects()
	return trigger.ProcessingResult{Status: status, Effects: &effects}
}

func unlinkTestTriggerResource(ctx context.Context, repo *TaskTriggerIntegrationPostgresRepository, id string, link trigger.ResourceLink) error {
	snapshot, err := repo.UnlinkSnapshot(ctx, id, link)
	if err != nil {
		return err
	}
	return repo.CommitUnlink(ctx, snapshot)
}

func seedAutomaticPullRequestLink(ctx context.Context, repo *TaskTriggerIntegrationPostgresRepository, id string, link trigger.ResourceLink) error {
	integration, err := repo.Get(ctx, id)
	if err != nil {
		return err
	}
	count, err := repo.queries.PutTaskTriggerResourceLink(ctx, sqlc.PutTaskTriggerResourceLinkParams{IntegrationID: id, WorkspaceID: integration.WorkspaceID, TaskID: link.TaskID, RepositoryID: link.RepositoryID, ResourceID: link.ResourceID, Provider: integration.Provider})
	if err != nil {
		return err
	}
	if count != 1 {
		return problem.ErrConflict
	}
	count, err = repo.queries.AssociatePullRequest(ctx, sqlc.AssociatePullRequestParams{WorkspaceID: integration.WorkspaceID, Provider: integration.Provider, RepositoryID: link.RepositoryID, ResourceID: link.ResourceID, TaskID: link.TaskID, HeadRepositoryID: link.RepositoryID, HeadBranch: "fixture/task"})
	if err != nil {
		return err
	}
	if count != 1 {
		return problem.ErrConflict
	}
	return nil
}

func ingestTestTriggerEvents(ctx context.Context, repo *TaskTriggerIntegrationPostgresRepository, i trigger.Integration, delivery string, events []trigger.Event) error {
	service, err := trigger.NewTaskTriggerIntegrationService(repo, nil, nil, nil)
	if err != nil {
		return err
	}
	return service.Ingest(ctx, i, delivery, events)
}

func saveTestTriggerConfiguration(ctx context.Context, repo *TaskTriggerIntegrationPostgresRepository, i trigger.Integration, create bool) (trigger.Integration, error) {
	workflows, err := repo.RuleWorkflows(ctx, i.WorkspaceID, i.Rules)
	if err != nil {
		return trigger.Integration{}, err
	}
	plan, err := trigger.PlanConfiguration(i, create, workflows)
	if err != nil {
		return trigger.Integration{}, err
	}
	return repo.Save(ctx, plan)
}
