package postgres

import (
	"context"
	"errors"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	"github.com/josemiguelmelo/orkest/internal/server/workflow"
	"github.com/josemiguelmelo/orkest/internal/server/workflowautomation"
)

func testTaskTransitionPolicyTransaction(t *testing.T, db *PostgresDatabase, orgID, workspaceID, actorID string) {
	t.Helper()
	ctx := context.Background()
	id, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	repo := NewTaskPostgresRepository(db)
	initial, err := createWorkflowTask(ctx, repo, id, workspaceID, taskmodel.TaskInput{WorkflowID: workspaceID, Title: "Policy transaction", Status: "todo"})
	if err != nil {
		t.Fatal(err)
	}
	command := workflowautomation.TransitionCommand{Input: workflowautomation.TransitionRequest{Action: "move", StageID: "in_review", StageVersion: initial.StageVersion, CommandID: id}}
	t.Run("outbox failure rolls back task and earlier events", func(t *testing.T) {
		_, err := NewTaskTransitionPostgresRepository(db).WithTaskTransition(ctx, actorID, orgID, workspaceID, id, command, func(snapshot workflowautomation.TransitionSnapshot, command workflowautomation.TransitionCommand) (workflowautomation.TransitionEffects, error) {
			effects, err := workflowautomation.PlanTaskTransition(snapshot, command)
			if err != nil {
				return effects, err
			}
			effects.Persistence.Events = append(effects.Persistence.Events, workflow.EventIntent{ID: id + ":invalid", Type: "invalid", StageVersion: effects.StageVersion})
			return effects, nil
		})
		if err == nil {
			t.Fatal("invalid outbox event accepted")
		}
		current, err := repo.Task(ctx, orgID, workspaceID, id)
		if err != nil || current.Status != initial.Status || current.StageVersion != initial.StageVersion {
			t.Fatalf("partial task update: %+v %v", current, err)
		}
		var events, commands int
		if err = db.pool.QueryRow(ctx, "SELECT count(*) FROM workflow_events WHERE workspace_id=$1 AND task_id=$2", workspaceID, id).Scan(&events); err != nil {
			t.Fatal(err)
		}
		if err = db.pool.QueryRow(ctx, "SELECT count(*) FROM workflow_commands WHERE workspace_id=$1 AND task_id=$2", workspaceID, id).Scan(&commands); err != nil {
			t.Fatal(err)
		}
		if events != 1 || commands != 0 {
			t.Fatalf("partial outbox effects: events=%d commands=%d", events, commands)
		}
	})
	rejected := errors.New("policy rejection")
	called := false
	_, err = NewTaskTransitionPostgresRepository(&PostgresDatabase{pool: repo.pool}).WithTaskTransition(ctx, actorID, orgID, workspaceID, id, command, func(snapshot workflowautomation.TransitionSnapshot, got workflowautomation.TransitionCommand) (workflowautomation.TransitionEffects, error) {
		called = true
		if snapshot.Task.ID != id || snapshot.Task.Status != initial.Status || snapshot.Workflow.ID != initial.WorkflowID || snapshot.RecordedCommand || got.Input != command.Input {
			t.Fatalf("incorrect locked snapshot or command: %+v, %+v", snapshot, got)
		}
		return workflowautomation.TransitionEffects{}, rejected
	})
	if !called || !errors.Is(err, rejected) {
		t.Fatalf("policy rejection: called=%v, error=%v", called, err)
	}
	unchanged, err := repo.Task(ctx, orgID, workspaceID, id)
	if err != nil || unchanged.Status != initial.Status || unchanged.StageVersion != initial.StageVersion {
		t.Fatalf("rejected policy changed task: %+v, %v", unchanged, err)
	}
	var count int
	if err = db.pool.QueryRow(ctx, "SELECT count(*) FROM workflow_commands WHERE workspace_id=$1 AND task_id=$2", workspaceID, id).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rejected policy persisted command: %d, %v", count, err)
	}
	// The same identity succeeds after rejection: neither intent nor effects committed.
	moved, err := newWorkflowTestService(repo).Transition(ctx, actorID, orgID, workspaceID, id, command.Input)
	if err != nil || moved.Status != "in_review" || moved.StageVersion != initial.StageVersion+1 {
		t.Fatalf("retry after rejection: %+v, %v", moved, err)
	}
	replay, err := newWorkflowTestService(repo).Transition(ctx, actorID, orgID, workspaceID, id, command.Input)
	if err != nil || replay.Status != moved.Status || replay.StageVersion != moved.StageVersion {
		t.Fatalf("stale-version replay: %+v, %v", replay, err)
	}
	command.Input.StageID = "done"
	if _, err = newWorkflowTestService(repo).Transition(ctx, actorID, orgID, workspaceID, id, command.Input); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatalf("identity reused with different input: %v", err)
	}
	if err = db.pool.QueryRow(ctx, "SELECT count(*) FROM workflow_events WHERE workspace_id=$1 AND task_id=$2", workspaceID, id).Scan(&count); err != nil || count != 2 {
		t.Fatalf("rejection/replay duplicated events: %d, %v", count, err)
	}
}
