package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	"github.com/josemiguelmelo/orkest/internal/server/workflow"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
	stage "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration"
)

func TestPostgresRepositoriesWorkflowStageIntegrations(t *testing.T) {
	dsn, err := connectionStringFromEnvironment()
	if err != nil {
		if os.Getenv("DATABASE_URL") == "" && os.Getenv("POSTGRES_HOST") == "" {
			t.Skip("PostgreSQL connection settings required")
		}
		t.Fatal(err)
	}
	ctx := context.Background()
	database, err := initialize(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	newID := func() string {
		id, e := ids.NewUUID()
		if e != nil {
			t.Fatal(e)
		}
		return id
	}
	user, err := NewUserPostgresRepository(database).RegisterUser(ctx, newID(), "stage-integration-test", "stage@example.test")
	if err != nil {
		t.Fatal(err)
	}
	defer deleteTestUser(t, database, user.ID)
	org, err := NewOrganizationPostgresRepository(database).CreateOrganization(ctx, newID(), user.ID, "stage integration tests")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = database.pool.Exec(ctx, `DELETE FROM organizations WHERE id=$1`, org.ID) }()
	workspaceID := newID()
	if _, err = NewWorkspacePostgresRepository(database).CreateWorkspace(ctx, workspaceID, org.ID, "integration workspace"); err != nil {
		t.Fatal(err)
	}
	q := sqlc.New(database.pool)
	coding, err := q.InsertCodingIntegration(ctx, sqlc.InsertCodingIntegrationParams{ID: newID(), WorkspaceID: workspaceID, Name: "github", ProviderName: "github", ProviderConfig: []byte(`{}`), EncryptedCredential: "encrypted-pat"})
	if err != nil {
		t.Fatal(err)
	}
	repoID := newID()
	_, err = q.InsertIntegratedCodeRepository(ctx, sqlc.InsertIntegratedCodeRepositoryParams{ID: repoID, WorkspaceID: workspaceID, ProviderName: "github", Url: "https://github.com/acme/backend.git", IntegrationID: pgtype.Text{String: coding.ID, Valid: true}, ProviderRepositoryID: pgtype.Text{String: "123", Valid: true}, DefaultBranch: "main", DisplayName: "backend"})
	if err != nil {
		t.Fatal(err)
	}
	repository := NewWorkflowStageIntegrationPostgresRepository(database)
	connection, err := repository.CreateConnection(ctx, stage.StoredConnection{Connection: stage.Connection{ID: newID(), WorkspaceID: workspaceID, Provider: "github_pr", Name: "PRs", Enabled: true, CodingIntegrationID: coding.ID, RepositoryIDs: []string{repoID}, Config: json.RawMessage(`{}`), ConfiguredBy: user.ID}, EncryptedSecret: "encrypted-webhook"})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := repository.ListConnections(ctx, workspaceID)
	if err != nil || len(listed) != 1 || listed[0].Version != 1 {
		t.Fatalf("connection list: %+v %v", listed, err)
	}
	stored, err := repository.CallbackConnection(ctx, connection.ID)
	if err != nil || stored.EncryptedSecret != "encrypted-webhook" {
		t.Fatalf("callback lookup: %v", err)
	}
	if _, err = repository.GetConnection(ctx, newID(), connection.ID); !errors.Is(err, problem.ErrNotFound) {
		t.Fatalf("cross-workspace get: %v", err)
	}
	invalid := stored
	invalid.Connection.RepositoryIDs = []string{newID()}
	if _, err = repository.UpdateConnection(ctx, invalid); !errors.Is(err, problem.ErrInvalid) {
		t.Fatalf("foreign repository update: %v", err)
	}

	workflows := NewWorkflowDefinitionPostgresRepository(database)
	definition, err := workflows.Definition(ctx, workspaceID, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	definition.ConfiguredBy = user.ID
	definition.Stages[1].ArtifactOutputs = []workflowmodel.ArtifactOutput{{Name: "pr", Kind: "code_review"}}
	definition.Stages[2].ArtifactInputs = []workflowmodel.ArtifactInput{{Name: "prs", SourceStageID: "in_progress", OutputName: "pr", Required: true}}
	definition.Rules = append(definition.Rules, workflowmodel.AutomationRule{ID: "review-feedback", Enabled: true, Event: workflowmodel.IntegrationAction, SourceStageID: "in_review", Action: workflowmodel.MoveTask, TargetStageID: "todo", Integration: &workflowmodel.IntegrationTrigger{ConnectionID: connection.ID, ActionID: "review_added", Parameters: json.RawMessage(`{"artifactInputName":"prs","reviewStates":["changes_requested"]}`)}})
	definition, err = workflows.Save(ctx, workspaceID, definition, workflow.PlanConfigurationChange)
	if err != nil {
		t.Fatal(err)
	}
	taskID := newID()
	_, err = database.pool.Exec(ctx, `INSERT INTO tasks(id,workspace_id,title,status,workflow_id) VALUES($1,$2,'Review task','in_review',$2)`, taskID, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.pool.Exec(ctx, `INSERT INTO task_repositories(workspace_id,task_id,repository_id) VALUES($1,$2,$3)`, workspaceID, taskID, repoID)
	if err != nil {
		t.Fatal(err)
	}
	artifactID, revisionID := newID(), newID()
	if err = q.CreateArtifact(ctx, sqlc.CreateArtifactParams{ID: artifactID, WorkspaceID: workspaceID, TaskID: taskID, Kind: "code_review", OutputName: "pr"}); err != nil {
		t.Fatal(err)
	}
	_, err = q.InsertArtifactRevision(ctx, sqlc.InsertArtifactRevisionParams{ID: revisionID, WorkspaceID: workspaceID, TaskID: taskID, ArtifactID: artifactID, Number: 1, Title: "PR", Content: []byte(`{"mediaType":"text/uri-list","url":"https://github.com/acme/backend/pull/42"}`), CreatedBy: user.ID, StageID: "in_progress", StageVersion: 1, IdempotencyKey: "pr", Request: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err = q.AdvanceArtifactRevision(ctx, artifactID); err != nil {
		t.Fatal(err)
	}
	candidates, err := repository.Candidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var candidate stage.Candidate
	for _, c := range candidates {
		if c.TaskID == taskID {
			candidate = c
			break
		}
	}
	if candidate.ObservationID == "" || len(candidate.AvailableArtifacts) != 1 || candidate.AvailableArtifacts[0].ID != revisionID || len(candidate.Repositories) != 1 || candidate.Repositories[0].ProviderRepositoryID != "123" {
		t.Fatalf("candidate inputs: %+v", candidate)
	}
	if err = repository.DeleteConnection(ctx, workspaceID, connection.ID); !errors.Is(err, problem.ErrConflict) {
		t.Fatalf("referenced connection deletion: %v", err)
	}
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	observation := stage.Observation{ID: candidate.ObservationID, TaskID: taskID, RuleID: candidate.RuleID, StageVersion: candidate.StageVersion, ActivatedAt: now, ReconciledAt: now, LastError: "subject_unavailable"}
	if err = repository.SaveObservation(ctx, candidate, observation); err != nil {
		t.Fatal(err)
	}
	subject := stage.Subject{Kind: "pull_request", ID: "456", RepositoryID: "123", URL: "https://github.com/acme/backend/pull/42", HeadSHA: "head"}
	observation.Subjects = []stage.Subject{subject}
	observation.RevisionIDs = []string{revisionID}
	observation.LastError = ""
	if err = repository.SaveObservation(ctx, candidate, observation); err != nil {
		t.Fatal("activate unresolved observation", err)
	}
	changed := observation
	changed.Subjects = []stage.Subject{{Kind: "pull_request", ID: "other", RepositoryID: "123"}}
	if err = repository.SaveObservation(ctx, candidate, changed); !errors.Is(err, problem.ErrConflict) {
		t.Fatalf("repin observation: %v", err)
	}
	refreshed := observation
	refreshed.Subjects = []stage.Subject{subject}
	refreshed.Subjects[0].HeadSHA = "new-head"
	if err = repository.SaveObservation(ctx, candidate, refreshed); err != nil {
		t.Fatal("refresh verified head", err)
	}
	event := stage.ProviderEvent{ID: "review:77", ActionID: "review_added", Subject: subject, OccurredAt: now.Add(time.Second), Facts: json.RawMessage(`{"state":"changes_requested"}`)}
	receipt := stage.CallbackResult{DeliveryID: "delivery-1", Events: []stage.ProviderEvent{event}}
	var wg sync.WaitGroup
	results := make(chan bool, 8)
	failures := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			duplicate, e := repository.RecordReceipt(ctx, connection, receipt)
			if e != nil {
				failures <- e
				return
			}
			results <- duplicate
		})
	}
	wg.Wait()
	close(results)
	close(failures)
	for e := range failures {
		t.Error(e)
	}
	fresh := 0
	for duplicate := range results {
		if !duplicate {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatalf("fresh receipts: %d", fresh)
	}
	// A distinct transport delivery cannot duplicate the same semantic review.
	receipt.DeliveryID = "delivery-2"
	if _, err = repository.RecordReceipt(ctx, connection, receipt); err != nil {
		t.Fatal(err)
	}
	events, err := repository.Events(ctx, candidate, observation)
	if err != nil || len(events) != 1 {
		t.Fatalf("semantic dedup: %d %v", len(events), err)
	}
	for range 2 {
		if err = repository.AdmitEvent(ctx, candidate, observation, event, "Review event"); err != nil {
			t.Fatal(err)
		}
	}
	events, err = repository.Events(ctx, candidate, observation)
	if err != nil || len(events) != 0 {
		t.Fatalf("admitted event returned: %d %v", len(events), err)
	}
	statuses, err := repository.ListStatus(ctx, workspaceID, connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	var eventID string
	for _, status := range statuses {
		if status.State == "pending" {
			eventID = status.ID
		}
	}
	if eventID == "" {
		t.Fatalf("missing effect status: %+v", statuses)
	}
	if err = repository.Retry(ctx, workspaceID, eventID); !errors.Is(err, problem.ErrConflict) {
		t.Fatalf("pending retry: %v", err)
	}
	_, err = database.pool.Exec(ctx, `UPDATE workflow_events SET processed_at=now(),processing_error='execution_busy' WHERE id=$1`, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if err = repository.Retry(ctx, workspaceID, eventID); err != nil {
		t.Fatal("retry blocked event", err)
	}
	var eventCount int
	if err = database.pool.QueryRow(ctx, `SELECT count(*) FROM workflow_events WHERE workspace_id=$1 AND event_type='integration_action'`, workspaceID).Scan(&eventCount); err != nil || eventCount != 1 {
		t.Fatalf("atomic callback dedup count: %d %v", eventCount, err)
	}
	// Configuration versions fence notifications verified before disable/rotation.
	stored.Connection = connection
	stored.Connection.Enabled = false
	updated, err := repository.UpdateConnection(ctx, stored)
	if err != nil || updated.Version != 2 {
		t.Fatalf("disable connection: %v", err)
	}
	if _, err = repository.RecordReceipt(ctx, connection, stage.CallbackResult{DeliveryID: "stale"}); !errors.Is(err, problem.ErrConflict) {
		t.Fatalf("stale receipt admitted: %v", err)
	}
	if err = repository.AdmitEvent(ctx, candidate, observation, stage.ProviderEvent{ID: "review:78", OccurredAt: now.Add(2 * time.Second)}, ""); !errors.Is(err, problem.ErrConflict) {
		t.Fatalf("stale observation admitted: %v", err)
	}
	if _, err = repository.UpdateConnection(ctx, stored); !errors.Is(err, problem.ErrConflict) {
		t.Fatalf("stale connection update: %v", err)
	}
}
