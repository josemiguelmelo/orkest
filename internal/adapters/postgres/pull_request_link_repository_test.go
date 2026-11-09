package postgres

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	observation "github.com/josemiguelmelo/orkest/internal/contracts/repositoryobservation"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	"github.com/josemiguelmelo/orkest/internal/server/pullrequestlink"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	trigger "github.com/josemiguelmelo/orkest/internal/server/tasktriggerintegration"
)

func testAutomaticPullRequestLinks(t *testing.T, db *PostgresDatabase, org, workspace, actor string) {
	t.Helper()
	ctx := context.Background()
	uuid := func() string {
		v, err := ids.NewUUID()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	tasks := NewTaskPostgresRepository(db)
	taskID, repositoryID, runID := uuid(), uuid(), uuid()
	if _, err := createWorkflowTask(ctx, tasks, taskID, workspace, taskmodel.TaskInput{WorkflowID: workspace, Title: "Automatic PR", Status: "todo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.pool.Exec(ctx, `INSERT INTO code_repositories(id,workspace_id,provider_name,url,provider_repository_id,default_branch) VALUES($1,$2,'github','https://github.com/owner/automatic','765','main')`, repositoryID, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := db.pool.Exec(ctx, `INSERT INTO task_repositories(workspace_id,task_id,repository_id) VALUES($1,$2,$3)`, workspace, taskID, repositoryID); err != nil {
		t.Fatal(err)
	}
	hooks := NewTaskTriggerIntegrationPostgresRepository(db)
	integration, err := saveTestTriggerConfiguration(ctx, hooks, trigger.Integration{ID: uuid(), WorkspaceID: workspace, OrganizationID: org, Name: "Automatic", Provider: "github", Config: json.RawMessage(`{}`), Enabled: true, ConfiguredBy: actor, EncryptedSecret: "encrypted"}, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := hooks.Delete(context.Background(), workspace, integration.ID); err != nil {
			t.Error(err)
		}
	})
	event := trigger.Event{Key: "review:automatic", Type: "pull_request.review_submitted", RepositoryID: "765", ResourceID: "8", HeadRepositoryID: "765", HeadBranch: "user-skill/my-choice", HeadCommitID: "abc123", OccurredAt: time.Now().Add(time.Second)}
	if err := ingestTestTriggerEvents(ctx, hooks, integration, "before-observation", []trigger.Event{event}); err != nil {
		t.Fatal(err)
	}
	linkRepository := NewPullRequestLinkRepository(db)
	links := pullrequestlink.NewPullRequestLinkService(linkRepository)
	if _, found, err := linkRepository.EventResolution(ctx, "another-integration", event.Key); err != nil || found {
		t.Fatalf("cross-integration resolution: %v %v", found, err)
	}
	if err := links.ResolveEvent(ctx, integration.ID, event.Key); err != nil {
		t.Fatal(err)
	}

	if _, err := db.pool.Exec(ctx, `UPDATE task_trigger_deliveries SET resolution_after=now() WHERE integration_id=$1`, integration.ID); err != nil {
		t.Fatal(err)
	}
	if found, err := links.ResolveNext(ctx); err != nil || !found {
		t.Fatalf("defer: %v %v", found, err)
	}
	deliveries, err := hooks.Deliveries(ctx, integration.ID)
	if err != nil || len(deliveries) != 1 || deliveries[0].Status != "awaiting_link" {
		t.Fatalf("lost unmatched event: %+v %v", deliveries, err)
	}
	if _, err := db.pool.Exec(ctx, `UPDATE task_trigger_deliveries SET resolution_after=now() WHERE integration_id=$1`, integration.ID); err != nil {
		t.Fatal(err)
	}
	stale, found, err := linkRepository.NextResolution(ctx)
	if err != nil || !found {
		t.Fatalf("snapshot: %v %v", found, err)
	}
	waitingPlan := pullrequestlink.PlanResolution(stale, time.Now())
	if committed, err := linkRepository.CommitResolution(ctx, stale, waitingPlan); err != nil || !committed {
		t.Fatalf("defer commit: %v %v", committed, err)
	}
	if committed, err := linkRepository.CommitResolution(ctx, stale, waitingPlan); err != nil || committed {
		t.Fatalf("superseded scheduling applied: %v %v", committed, err)
	}
	if _, err := db.pool.Exec(ctx, `UPDATE task_trigger_deliveries SET resolution_after=now() WHERE integration_id=$1`, integration.ID); err != nil {
		t.Fatal(err)
	}
	stale, found, err = linkRepository.NextResolution(ctx)
	if err != nil || !found {
		t.Fatal(err)
	}
	stalePlan := pullrequestlink.PlanResolution(stale, time.Now())
	launches := NewAgentLaunchPostgresRepository(db, nil)
	if err := launches.CreateLaunch(ctx, orchestrationmodel.Launch{RunID: runID, OrganizationID: org, WorkspaceID: workspace, TaskID: taskID, Status: "ready"}); err != nil {
		t.Fatal(err)
	}
	snapshot := observation.Snapshot{Checkout: observation.Checkout{RepositoryID: repositoryID, Path: "repositories/github/automatic", DefaultBranch: "main"}, Branches: []observation.Branch{{Name: event.HeadBranch, CommitID: event.HeadCommitID, EligibleForLinking: true}}}
	if err := launches.SaveEvent(ctx, orchestrationmodel.Event{RunID: runID, Sequence: 1, Payload: orchestrationmodel.AgentActivityEvent{At: time.Now(), Type: orchestrationmodel.EventRepositoryObserved, RepositoryObservation: &snapshot}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.pool.Exec(ctx, `UPDATE task_trigger_deliveries SET resolution_after=now() WHERE integration_id=$1`, integration.ID); err != nil {
		t.Fatal(err)
	}
	if committed, err := linkRepository.CommitResolution(ctx, stale, stalePlan); err != nil || committed {
		t.Fatalf("stale evidence applied: %v %v", committed, err)
	}
	var workers sync.WaitGroup
	failures := make(chan error, 2)
	for index := range 2 {
		workers.Go(func() {
			if index == 0 {
				_, err := links.ResolveNext(ctx)
				failures <- err
				return
			}
			failures <- links.ResolveEvent(ctx, integration.ID, event.Key)
		})
	}
	workers.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	linked, err := hooks.Links(ctx, integration.ID)
	if err != nil || len(linked) != 1 || linked[0].TaskID != taskID {
		t.Fatalf("automatic link: %+v %v", linked, err)
	}
	deliveries, err = hooks.Deliveries(ctx, integration.ID)
	if err != nil || deliveries[0].Status != "pending" {
		t.Fatalf("event not bound: %+v %v", deliveries, err)
	}
	if err := unlinkTestTriggerResource(ctx, hooks, integration.ID, linked[0]); err != nil {
		t.Fatal(err)
	}
	event.Key = "review:after-unlink"
	if err := ingestTestTriggerEvents(ctx, hooks, integration, "suppressed", []trigger.Event{event}); err != nil {
		t.Fatal(err)
	}
	if _, err := links.ResolveNext(ctx); err != nil {
		t.Fatal(err)
	}
	linked, err = hooks.Links(ctx, integration.ID)
	if err != nil || len(linked) != 0 {
		t.Fatalf("unlink automatically recreated: %+v %v", linked, err)
	}

	// Evidence from a task without the repository selected cannot compete.
	otherTask, otherRun := uuid(), uuid()
	if _, err := createWorkflowTask(ctx, tasks, otherTask, workspace, taskmodel.TaskInput{WorkflowID: workspace, Title: "Other task", Status: "todo"}); err != nil {
		t.Fatal(err)
	}
	if err := launches.CreateLaunch(ctx, orchestrationmodel.Launch{RunID: otherRun, OrganizationID: org, WorkspaceID: workspace, TaskID: otherTask, Status: "ready"}); err != nil {
		t.Fatal(err)
	}
	if err := launches.SaveEvent(ctx, orchestrationmodel.Event{RunID: otherRun, Sequence: 1, Payload: orchestrationmodel.AgentActivityEvent{At: time.Now(), Type: orchestrationmodel.EventRepositoryObserved, RepositoryObservation: &snapshot}}); err != nil {
		t.Fatal(err)
	}
	event.Key, event.ResourceID = "review:not-selected", "9"
	if err := ingestTestTriggerEvents(ctx, hooks, integration, "not-selected", []trigger.Event{event}); err != nil {
		t.Fatal(err)
	}
	if _, err := links.ResolveNext(ctx); err != nil {
		t.Fatal(err)
	}
	linked, err = hooks.Links(ctx, integration.ID)
	if err != nil || len(linked) != 1 || linked[0].TaskID != taskID {
		t.Fatalf("unselected repository accepted: %+v %v", linked, err)
	}
	// Once both tasks supply selected repository evidence, fail visibly instead of guessing.
	if _, err := db.pool.Exec(ctx, `INSERT INTO task_repositories(workspace_id,task_id,repository_id) VALUES($1,$2,$3)`, workspace, otherTask, repositoryID); err != nil {
		t.Fatal(err)
	}
	event.Key, event.ResourceID = "review:conflicting", "10"
	if err := ingestTestTriggerEvents(ctx, hooks, integration, "conflict", []trigger.Event{event}); err != nil {
		t.Fatal(err)
	}
	if _, err := links.ResolveNext(ctx); err != nil {
		t.Fatal(err)
	}
	deliveries, err = hooks.Deliveries(ctx, integration.ID)
	if err != nil {
		t.Fatal(err)
	}
	conflictFound := false
	for _, delivery := range deliveries {
		if delivery.Event.Key == event.Key {
			conflictFound = delivery.Status == "blocked" && delivery.Diagnostic == "conflicting_branch_ownership"
		}
	}
	if !conflictFound {
		t.Fatalf("conflicting tasks were not blocked: %+v", deliveries)
	}
	// Existing PR ownership does not make an old event eligible for a later stage.
	if _, err := db.pool.Exec(ctx, `INSERT INTO workflow_events(id,workspace_id,task_id,stage_version,actor_id,event_type,causation_id,correlation_id,occurred_at) VALUES($1,$2,$3,1,'fixture','stage_entered',$1,$1,now()+interval '10 seconds')`, uuid(), workspace, taskID); err != nil {
		t.Fatal(err)
	}
	event.Key, event.ResourceID = "review:stale", "9"
	if _, err := db.pool.Exec(ctx, `DELETE FROM task_trigger_resource_links WHERE integration_id=$1 AND resource_id='9'`, integration.ID); err != nil {
		t.Fatal(err)
	}
	if err := ingestTestTriggerEvents(ctx, hooks, integration, "stale", []trigger.Event{event}); err != nil {
		t.Fatal(err)
	}
	if _, err := links.ResolveNext(ctx); err != nil {
		t.Fatal(err)
	}
	deliveries, err = hooks.Deliveries(ctx, integration.ID)
	if err != nil {
		t.Fatal(err)
	}
	staleFound := false
	for _, delivery := range deliveries {
		if delivery.Event.Key == event.Key {
			staleFound = delivery.Status == "ignored" && delivery.Diagnostic == "stale_stage_entry"
		}
	}
	if !staleFound {
		t.Fatalf("stale event became executable: %+v", deliveries)
	}
	// Organization-wide webhooks may include unrelated repository identities.
	otherWorkspace := uuid()
	if _, err := NewWorkspacePostgresRepository(db).CreateWorkspace(ctx, otherWorkspace, org, "Other webhook workspace"); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		id, workspace, provider string
	}{
		{"901", workspace, "gitlab_cloud"},
		{"902", otherWorkspace, "github"},
	} {
		if _, err := db.pool.Exec(ctx, `INSERT INTO code_repositories(id,workspace_id,provider_name,url,provider_repository_id) VALUES($1,$2,$3,$4,$5)`, uuid(), fixture.workspace, fixture.provider, "https://example.test/owner/"+fixture.id, fixture.id); err != nil {
			t.Fatal(err)
		}
	}
	for _, repository := range []string{"900", "901", "902"} {
		unrelated := event
		unrelated.Key, unrelated.RepositoryID, unrelated.HeadRepositoryID = "review:unrelated:"+repository, repository, repository
		if err := ingestTestTriggerEvents(ctx, hooks, integration, "unrelated-"+repository, []trigger.Event{unrelated}); err != nil {
			t.Fatal(err)
		}
		if err := links.ResolveEvent(ctx, integration.ID, unrelated.Key); err != nil {
			t.Fatal(err)
		}
		deliveries, err := hooks.Deliveries(ctx, integration.ID)
		if err != nil {
			t.Fatal(err)
		}
		ignored := false
		for _, delivery := range deliveries {
			if delivery.Event.Key == unrelated.Key {
				ignored = delivery.Status == "ignored" && delivery.Diagnostic == "repository_not_registered"
			}
		}
		if !ignored {
			t.Fatalf("unrelated repository %s was not ignored: %+v", repository, deliveries)
		}
	}

	target, found, err := linkRepository.ClaimDiscovery(ctx)
	if err != nil || !found || target.Since.IsZero() {
		t.Fatalf("discovery claim: %+v %v %v", target, found, err)
	}
}
