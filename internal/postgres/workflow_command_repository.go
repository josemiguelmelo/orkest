package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/task"
)

func (r *TaskPostgresRepository) DeliverWorkflowCommand(ctx context.Context, deliver func(context.Context, task.WorkflowCommand) (bool, error)) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	row, err := q.PendingWorkflowCommand(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	c := task.WorkflowCommand{ID: row.ID, OrganizationID: row.OrganizationID, WorkspaceID: row.WorkspaceID, TaskID: row.TaskID, ActorID: row.ActorID, StageVersion: row.StageVersion}
	if err = json.Unmarshal(row.Options, &c.Options); err != nil {
		return false, err
	}
	delivered, err := deliver(ctx, c)
	if err != nil {
		return false, err
	}
	state, workState := "failed", task.WorkStateFailed
	if delivered {
		state, workState = "delivered", task.WorkStateRunning
	}
	if err = q.SetWorkflowCommandState(ctx, sqlc.SetWorkflowCommandStateParams{ID: c.ID, State: state}); err != nil {
		return false, err
	}
	if err = q.SetQueuedStageState(ctx, sqlc.SetQueuedStageStateParams{WorkspaceID: c.WorkspaceID, ID: c.TaskID, StageVersion: c.StageVersion, StageWorkState: string(workState)}); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
