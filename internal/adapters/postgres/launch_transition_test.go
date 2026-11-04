package postgres

import (
	"context"
	"sync"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
)

func testLaunchTransitionFencing(t *testing.T, db *PostgresDatabase, orgID, workspaceID, taskID string) {
	ctx := context.Background()
	repo := NewAgentLaunchPostgresRepository(db, nil)
	for _, final := range []orchestration.LaunchStatus{orchestration.LaunchCompleted, orchestration.LaunchFailed} {
		runID, err := ids.NewUUID()
		if err != nil {
			t.Fatal(err)
		}
		launch := orchestration.Launch{RunID: runID, OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: taskID, Status: orchestration.LaunchReady}
		if err = repo.CreateLaunch(ctx, launch); err != nil {
			t.Fatal(err)
		}
		pending := orchestration.LaunchCompletedPendingClose
		opposite := orchestration.LaunchFailedPendingClose
		if final == orchestration.LaunchFailed {
			pending, opposite = opposite, pending
		}
		if err = repo.SetLaunchStatus(ctx, orchestration.PlanLaunchStatus(runID, pending)); err != nil {
			t.Fatal(err)
		}
		var work sync.WaitGroup
		failures := make(chan error, 10)
		// Competing status updates cannot reverse terminal intent.
		for range 5 {
			work.Go(func() { failures <- repo.UpdateLaunch(ctx, launch) })
			work.Go(func() { failures <- repo.SetLaunchStatus(ctx, orchestration.PlanLaunchStatus(runID, opposite)) })
		}
		work.Wait()
		close(failures)
		for err = range failures {
			if err != nil {
				t.Fatal(err)
			}
		}
		var status string
		if err = db.pool.QueryRow(ctx, "SELECT status FROM agent_launches WHERE run_id=$1", runID).Scan(&status); err != nil || status != string(pending) {
			t.Fatal("pending status regressed", status, err)
		}
		if err = repo.SetLaunchStatus(ctx, orchestration.PlanLaunchStatus(runID, final)); err != nil {
			t.Fatal(err)
		}
		// Late event replay must retain final state, including the legacy failed state.
		event := orchestration.Event{RunID: runID, Sequence: 1, Payload: orchestration.AgentActivityEvent{RunID: runID, Sequence: 1, Type: orchestration.EventRunCompleted}}
		if err = repo.SaveTerminalEvent(ctx, event, orchestration.LaunchCompletedPendingClose); err != nil {
			t.Fatal(err)
		}
		if err = repo.SetLaunchStatus(ctx, orchestration.PlanLaunchStatus(runID, orchestration.LaunchCompleted)); err != nil {
			t.Fatal(err)
		}
		if err = db.pool.QueryRow(ctx, "SELECT status FROM agent_launches WHERE run_id=$1", runID).Scan(&status); err != nil || status != string(final) {
			t.Fatal("final status regressed", status, err)
		}
	}
}
