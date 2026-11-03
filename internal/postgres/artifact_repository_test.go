package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/artifact"
	"github.com/josemiguelmelo/orkest/internal/core/ids"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
)

func testTaskArtifacts(t *testing.T, db *PostgresDatabase, orgID, workspaceID, taskID, userID string) {
	t.Helper()
	ctx := context.Background()
	isolatedID, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.pool.Exec(ctx, "INSERT INTO tasks(id,workspace_id,workflow_id,title,status) SELECT $1,workspace_id,workflow_id,'Artifacts',status FROM tasks WHERE workspace_id=$2 AND id=$3", isolatedID, workspaceID, taskID); err != nil {
		t.Fatal(err)
	}
	taskID = isolatedID
	repo := NewArtifactPostgresRepository(db)
	scope := artifact.Scope{UserID: userID, OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: taskID}
	text := "# Implementation plan"
	key, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	input := artifact.PublishInput{Kind: "plan", Title: "Implementation plan", OutputName: "plan", IdempotencyKey: key, Content: artifact.Content{MediaType: "text/markdown", Text: &text}}
	first, err := repo.Publish(ctx, scope, input)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := repo.Publish(ctx, scope, input)
	if err != nil || replay.ID != first.ID {
		t.Fatalf("replay: %+v %v", replay, err)
	}
	input.Title = "Conflicting retry"
	if _, err = repo.Publish(ctx, scope, input); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatalf("conflicting key: %v", err)
	}
	input.Title = "Revised plan"
	input.ArtifactID = first.ArtifactID
	input.ExpectedRevision = 1
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Go(func() {
			candidate := input
			candidate.IdempotencyKey, _ = ids.NewUUID()
			_, e := repo.Publish(ctx, scope, candidate)
			results <- e
		})
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, modelerrors.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent publications: successes=%d conflicts=%d", successes, conflicts)
	}
	revisions, err := repo.Revisions(ctx, scope, first.ArtifactID)
	if err != nil || len(revisions) != 2 || revisions[0].Title != "Implementation plan" || revisions[1].Number != 2 {
		t.Fatalf("history: %+v %v", revisions, err)
	}
	wrong := scope
	wrong.TaskID = "other-task"
	if _, err = repo.Revisions(ctx, wrong, first.ArtifactID); !errors.Is(err, modelerrors.ErrNotFound) {
		t.Fatalf("scope leak: %v", err)
	}
	scope.RunID = "unrelated-run"
	input.IdempotencyKey, _ = ids.NewUUID()
	input.ExpectedRevision = 2
	if _, err = repo.Publish(ctx, scope, input); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatalf("unbound run: %v", err)
	}
	// Pin the original immutable revision to an actual run, then publish another revision.
	runID, _ := ids.NewUUID()
	if err = db.pool.QueryRow(ctx, "SELECT 1 FROM tasks WHERE workspace_id=$1 AND id=$2", workspaceID, taskID).Scan(new(int)); err != nil {
		t.Fatal(err)
	}
	q := sqlc.New(db.pool)
	if err = q.InsertAgentLaunch(ctx, sqlc.InsertAgentLaunchParams{RunID: runID, OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: taskID, Status: "ready"}); err != nil {
		t.Fatal(err)
	}
	scope.RunID = runID
	produced, err := repo.Publish(ctx, scope, input)
	if err != nil || produced.ProducingRunID != runID {
		t.Fatalf("run publication: %+v %v", produced, err)
	}
	count, err := q.PinRunArtifactInput(ctx, sqlc.PinRunArtifactInputParams{RunID: runID, ID: first.ID})
	if err != nil || count != 1 {
		t.Fatalf("pin: %d %v", count, err)
	}
	var pinned string
	if err = db.pool.QueryRow(ctx, "SELECT r.title FROM run_artifact_inputs i JOIN task_artifact_revisions r ON r.id=i.revision_id WHERE i.run_id=$1", runID).Scan(&pinned); err != nil || pinned != "Implementation plan" {
		t.Fatalf("pinned revision changed: %q %v", pinned, err)
	}
	if err = q.UpdateAgentLaunch(ctx, sqlc.UpdateAgentLaunchParams{RunID: runID, Status: "completed"}); err != nil {
		t.Fatal(err)
	}
	input.IdempotencyKey, _ = ids.NewUUID()
	input.ExpectedRevision = 3
	if _, err = repo.Publish(ctx, scope, input); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatalf("ended run publication: %v", err)
	}
}
