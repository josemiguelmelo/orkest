package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	agentmodel "github.com/josemiguelmelo/orkest/internal/server/agent/model"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
	runmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	jobmodel "github.com/josemiguelmelo/orkest/internal/server/scheduledjob/model"
)

func testScheduledJobs(t *testing.T, db *PostgresDatabase, org, user, agent, revokedActor string) {
	t.Helper()
	ctx := context.Background()
	workspace, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = NewWorkspacePostgresRepository(db).CreateWorkspace(ctx, workspace, org, "scheduled jobs tests"); err != nil {
		t.Fatal(err)
	}
	localAgent, err := NewAgentPostgresRepository(db, nil).CreateAgent(ctx, agent+"-scheduled", workspace, agentmodel.AgentDefinition{Name: "job agent", Runtime: "opencode", Provider: "test", Model: "model", PermissionPolicy: "ask"})
	if err != nil {
		t.Fatal(err)
	}
	agent = localAgent.ID
	repo := NewScheduledJobPostgresRepository(db)
	launches := NewAgentLaunchPostgresRepository(db, nil)
	create := func(name string, in jobmodel.Input) jobmodel.Job {
		t.Helper()
		id, e := ids.NewUUID()
		if e != nil {
			t.Fatal(e)
		}
		job, e := repo.Save(ctx, user, org, workspace, id, 0, in, time.Now().Add(time.Hour))
		if e != nil {
			t.Fatalf("create %s: %v", name, e)
		}
		return job
	}
	input := jobmodel.Input{Name: "review", Instructions: "Review repositories", AgentID: agent, RepositoryScope: "none", RepositoryIDs: []string{}, Enabled: true, MaxDurationMinutes: 60, Schedule: jobmodel.Schedule{Frequency: "daily", TimeZone: "UTC", Hour: time.Now().UTC().Hour(), Minute: time.Now().UTC().Minute()}}
	job := create("daily", input)
	repeated, err := repo.Save(ctx, user, org, workspace, job.ID, 0, input, time.Now())
	if err != nil || repeated.ID != job.ID {
		t.Fatalf("creation replay: %v", err)
	}
	altered := input
	altered.Instructions = "different"
	if _, err = repo.Save(ctx, user, org, workspace, job.ID, 0, altered, time.Now()); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatalf("changed creation replay: %v", err)
	}
	if _, err = repo.Get(ctx, "other-org", workspace, job.ID); !errors.Is(err, modelerrors.ErrNotFound) {
		t.Fatalf("cross-org read: %v", err)
	}
	if _, err = db.pool.Exec(ctx, "UPDATE scheduled_jobs SET next_due_at=date_trunc('minute',now()) WHERE id=$1", job.ID); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	failures := make(chan error, 8)
	for range 8 {
		group.Go(func() {
			_, e := repo.Materialize(ctx)
			if e != nil {
				failures <- e
			}
		})
	}
	group.Wait()
	close(failures)
	for e := range failures {
		t.Fatal(e)
	}
	runs, err := repo.Runs(ctx, org, workspace, job.ID, 100, 0)
	if err != nil || len(runs) != 1 || runs[0].State != "pending" {
		t.Fatalf("materialized once: %v %#v", err, runs)
	}
	var count int
	if err = db.pool.QueryRow(ctx, "SELECT count(*) FROM tasks WHERE workspace_id=$1", workspace).Scan(&count); err != nil || count != 0 {
		t.Fatalf("job created workflow task: %d %v", count, err)
	}
	replay, err := repo.Enqueue(ctx, user, org, workspace, job.ID, "manual-overlap")
	if err != nil || replay.State != "skipped" {
		t.Fatalf("overlap: %v %#v", err, replay)
	}
	same, err := repo.Enqueue(ctx, user, org, workspace, job.ID, "manual-overlap")
	if err != nil || same.ID != replay.ID {
		t.Fatalf("manual replay: %v", err)
	}
	second := create("second", input)
	if _, err = repo.Enqueue(ctx, user, org, workspace, second.ID, "second"); err != nil {
		t.Fatal(err)
	}
	claims := make(chan jobmodel.Run, 2)
	failures = make(chan error, 2)
	for _, token := range []string{"first-a", "first-b"} {
		group.Go(func() {
			r, found, e := repo.Claim(ctx, token)
			if e != nil {
				failures <- e
			} else if found {
				claims <- r
			}
		})
	}
	group.Wait()
	close(claims)
	close(failures)
	for e := range failures {
		t.Fatal(e)
	}
	var first jobmodel.Run
	for c := range claims {
		if first.ID != "" {
			t.Fatal("two runs acquired one workspace quota")
		}
		first = c
	}
	if first.ID == "" {
		t.Fatal("no capacity winner")
	}
	if _, err = db.pool.Exec(ctx, "UPDATE scheduled_job_runs SET claim_until=now()-interval '1 second' WHERE id=$1", first.ID); err != nil {
		t.Fatal(err)
	}
	successor, found, err := repo.Claim(ctx, "successor")
	if err != nil || !found || successor.ID != first.ID || successor.ClaimAttempt != 2 {
		t.Fatalf("claim recovery: %v %#v", err, successor)
	}
	config := runmodel.RunConfig{RuntimeConfig: runmodel.RuntimeConfig{OrganizationID: org, WorkspaceID: workspace, TaskID: first.ID, TaskSessionID: first.ID + ":session", RunID: first.RunID, Runtime: "opencode", Agent: runmodel.AgentDefinition{Name: "job", Instructions: "review"}, Model: runmodel.ModelSelection{Provider: "test", ID: "model"}}, InitialCommandID: first.CommandID, InitialPrompt: "review"}
	if err = launches.ReserveJobSessionRun(ctx, first, config); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatalf("stale claimant reserved: %v", err)
	}
	if err = launches.ReserveJobSessionRun(ctx, successor, config); err != nil {
		t.Fatalf("reserve successor: %v", err)
	}
	accepted, err := launches.TaskCommand(ctx, org, workspace, first.ID, first.CommandID, "")
	if err != nil || !accepted {
		t.Fatalf("acceptance not durable: %v", err)
	}
	cancelled, err := repo.CancelPending(ctx, org, workspace, first.JobID, first.ID)
	if err != nil || cancelled {
		t.Fatalf("cancel released accepted run: %v", err)
	}
	if err = repo.FinishClaim(ctx, first, "failed", "stale"); err != nil {
		t.Fatal(err)
	}
	current, err := repo.Run(ctx, org, workspace, first.JobID, first.ID)
	if err != nil || current.Reason == "stale" {
		t.Fatal("stale acknowledgement changed successor")
	}
	if err = repo.FinishClaim(ctx, successor, "running", ""); err != nil {
		t.Fatal(err)
	}
	if err = launches.SetLaunchStatus(ctx, orchestration.PlanLaunchStatus(first.RunID, runmodel.LaunchFailedPendingClose)); err != nil {
		t.Fatal(err)
	}
	if _, found, err = repo.Claim(ctx, "must-wait"); err != nil || found {
		t.Fatalf("cleanup released capacity early: %v", err)
	}
	if err = launches.SetLaunchStatus(ctx, orchestration.PlanLaunchStatus(first.RunID, runmodel.LaunchFailed)); err != nil {
		t.Fatal(err)
	}
	current, err = repo.Run(ctx, org, workspace, first.JobID, first.ID)
	if err != nil || current.State != "cancelled" {
		t.Fatalf("terminal job result: %v %#v", err, current)
	}
	if err = repo.FinishClaim(ctx, successor, "running", "late acknowledgement"); err != nil {
		t.Fatal(err)
	}
	afterAck, err := repo.Run(ctx, org, workspace, first.JobID, first.ID)
	if err != nil || afterAck.State != "cancelled" {
		t.Fatal("late acknowledgement reversed terminal state")
	}
	if _, found, err = repo.Claim(ctx, "after-cleanup"); err != nil || !found {
		t.Fatalf("resolved ownership did not release capacity: %v", err)
	}
	if err = db.pool.QueryRow(ctx, "SELECT count(*) FROM workflow_events WHERE workspace_id=$1", workspace).Scan(&count); err != nil || count != 0 {
		t.Fatalf("job emitted workflow events: %d %v", count, err)
	}
	// Exact occurrence scope survives later repository registration.
	all := input
	all.RepositoryScope = "all"
	allJob := create("all", all)
	repositoryID, e := ids.NewUUID()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.pool.Exec(ctx, "INSERT INTO code_repositories(id,workspace_id,provider_name,url) VALUES($1,$2,'github','https://github.com/test/one.git')", repositoryID, workspace); e != nil {
		t.Fatal(e)
	}
	allRun, e := repo.Enqueue(ctx, user, org, workspace, allJob.ID, "scope")
	if e != nil || len(allRun.RepositoryIDs) != 1 || allRun.RepositoryIDs[0] != repositoryID {
		t.Fatalf("all snapshot: %#v %v", allRun, e)
	}
	if _, e = db.pool.Exec(ctx, "INSERT INTO code_repositories(id,workspace_id,provider_name,url) VALUES($1,$2,'github','https://github.com/test/two.git')", repositoryID+"-second", workspace); e != nil {
		t.Fatal(e)
	}
	allRun, e = repo.Run(ctx, org, workspace, allJob.ID, allRun.ID)
	if e != nil || len(allRun.RepositoryIDs) != 1 {
		t.Fatal("snapshotted scope changed")
	}
	// Outbox insert failure rolls back session and launch reservation together.
	rollbackJob := create("rollback", input)
	rollbackRun, e := repo.Enqueue(ctx, user, org, workspace, rollbackJob.ID, "rollback")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.pool.Exec(ctx, "UPDATE scheduled_job_runs SET state='cancelled' WHERE workspace_id=$1 AND state IN ('pending','starting') AND id<>$2", workspace, rollbackRun.ID); e != nil {
		t.Fatal(e)
	}
	claim, found, e := repo.Claim(ctx, "rollback-claim")
	if e != nil || !found || claim.ID != rollbackRun.ID {
		t.Fatalf("rollback claim %v %#v", e, claim)
	}
	broken := config
	broken.TaskID = claim.ID
	broken.TaskSessionID = claim.ID + ":session"
	broken.RunID = claim.RunID
	broken.InitialCommandID = claim.CommandID
	broken.InitialPrompt = ""
	// A conflicting session identity forces reservation rollback before launch/outbox.
	if e = sqlc.New(db.pool).InsertTaskAgentSession(ctx, sqlc.InsertTaskAgentSessionParams{ID: broken.TaskSessionID, OrganizationID: org, WorkspaceID: workspace, TaskID: "conflicting-owner", Configuration: []byte(`{}`)}); e != nil {
		t.Fatal(e)
	}
	if e = launches.ReserveJobSessionRun(ctx, claim, broken); e == nil {
		t.Fatal("conflicting session reservation succeeded")
	}
	if e = db.pool.QueryRow(ctx, "SELECT count(*) FROM agent_launches WHERE run_id=$1", claim.RunID).Scan(&count); e != nil || count != 0 {
		t.Fatalf("partial launch survived rollback: %d %v", count, e)
	}
	// A saved authorization cannot survive the loss of workspace mutation access.
	revokedID, e := ids.NewUUID()
	if e != nil {
		t.Fatal(e)
	}
	revokedJob, e := repo.Save(ctx, revokedActor, org, workspace, revokedID, 0, input, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	rejected, e := repo.Enqueue(ctx, revokedActor, org, workspace, revokedJob.ID, "revoked")
	if e != nil || rejected.State != "needs_attention" || rejected.Reason != "authorization_revoked" {
		t.Fatalf("revoked authorization: %#v %v", rejected, e)
	}
	paused, e := repo.Get(ctx, org, workspace, revokedJob.ID)
	if e != nil || paused.Enabled {
		t.Fatalf("revoked job was not suspended: %v", e)
	}
	// Cleanup is test-owned and never terminates external processes.
	if _, e = db.pool.Exec(ctx, "DELETE FROM agent_launches WHERE workspace_id=$1", workspace); e != nil {
		t.Fatal(e)
	}
	if _, e = db.pool.Exec(ctx, "DELETE FROM task_agent_sessions WHERE workspace_id=$1", workspace); e != nil {
		t.Fatal(e)
	}
}
