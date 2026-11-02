package postgres

import (
	"context"
	"sync"
	"testing"

	workflowautomation "github.com/josemiguelmelo/orkest/internal/workflowautomation"

	"github.com/josemiguelmelo/orkest/internal/core/ids"
	"github.com/josemiguelmelo/orkest/internal/task"
	"github.com/josemiguelmelo/orkest/internal/workflow"
)

func testWorkflowEventAdmission(t *testing.T, db *PostgresDatabase, orgID, workspaceID, actorID, agentID string) {
	t.Helper()
	ctx := context.Background()
	newID := func() string {
		id, err := ids.NewUUID()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	definitions := NewWorkflowDefinitionPostgresRepository(db)
	d := workflow.DefaultDefinition("Event admission")
	d.ID = newID()
	d.ConfiguredBy = actorID
	d.Stages[1].ResponsibleAgentID = agentID
	if _, err := definitions.Create(ctx, workspaceID, d, workflow.PlanConfigurationChange); err != nil {
		t.Fatal(err)
	}
	tasks := NewTaskPostgresRepository(db)
	worker := newWorkflowTestWorker(tasks)
	create := func() task.Task {
		v, err := createWorkflowTask(ctx, tasks, newID(), workspaceID, task.TaskInput{WorkflowID: d.ID, Title: "Event admission", Status: "in_progress", ActorID: actorID})
		if err != nil {
			t.Fatal(err)
		}
		if !v.AutomationPending {
			t.Fatal("committed entry event not visible")
		}
		return v
	}
	startCount := func(id string) int {
		var count int
		if err := db.pool.QueryRow(ctx, "SELECT count(*) FROM workflow_commands WHERE workspace_id=$1 AND task_id=$2 AND options->>'action'='start'", workspaceID, id).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	t.Run("obsolete entry does not start", func(t *testing.T) {
		v := create()
		_, err := newWorkflowTestService(tasks).Transition(ctx, actorID, orgID, workspaceID, v.ID, workflowautomation.TransitionRequest{Action: "move", StageID: "in_review", StageVersion: v.StageVersion, CommandID: newID()})
		if err != nil {
			t.Fatal(err)
		}
		processWorkflowCommands(t, ctx, worker)
		if startCount(v.ID) != 0 {
			t.Fatal("old stage entry started work")
		}
	})
	t.Run("explicit start supersedes pending entry", func(t *testing.T) {
		v := create()
		_, err := newWorkflowTestService(tasks).Transition(ctx, actorID, orgID, workspaceID, v.ID, workflowautomation.TransitionRequest{Action: "start", StageVersion: v.StageVersion, CommandID: newID()})
		if err != nil {
			t.Fatal(err)
		}
		processWorkflowCommands(t, ctx, worker)
		if startCount(v.ID) != 1 {
			t.Fatal("entry duplicated explicit start")
		}
	})
	t.Run("competing event workers enqueue once", func(t *testing.T) {
		v := create()
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for range 2 {
			wg.Go(func() { _, err := newTaskWorkflowService(tasks).ProcessNextEvent(ctx); results <- err })
		}
		wg.Wait()
		close(results)
		for err := range results {
			if err != nil {
				t.Fatal(err)
			}
		}
		processWorkflowCommands(t, ctx, worker)
		got, err := tasks.Task(ctx, orgID, workspaceID, v.ID)
		if err != nil || got.AutomationPending || startCount(v.ID) != 1 {
			t.Fatal("event delivery not idempotent", got, err)
		}
		var matched bool
		if err = db.pool.QueryRow(ctx, "SELECT processed_at IS NOT NULL AND matched_rule_id IS NOT NULL AND configuration_version=$3 FROM workflow_events WHERE workspace_id=$1 AND task_id=$2", workspaceID, v.ID, d.Version).Scan(&matched); err != nil || !matched {
			t.Fatal("event audit missing", err)
		}
	})
	t.Run("cancelled delivery remains pending after restart", func(t *testing.T) {
		v := create()
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := newTaskWorkflowService(tasks).ProcessNextEvent(cancelled); err == nil {
			t.Fatal("cancelled processing succeeded")
		}
		if startCount(v.ID) != 0 {
			t.Fatal("cancelled event had effects")
		}
		processWorkflowCommands(t, ctx, newWorkflowTestWorker(NewTaskPostgresRepository(db)))
		if startCount(v.ID) != 1 {
			t.Fatal("replacement worker lost durable entry")
		}
	})
}
