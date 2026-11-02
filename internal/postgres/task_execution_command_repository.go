package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	workflowautomation "github.com/josemiguelmelo/orkest/internal/workflowautomation"

	"github.com/jackc/pgx/v5"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
)

func (r *ExecutionCommandPostgresRepository) DeliverExecutionCommand(ctx context.Context, deliver func(context.Context, workflowautomation.ExecutionCommand, workflowautomation.ExecutionHealthReader) (workflowautomation.ExecutionDeliveryEffects, error)) (bool, error) {
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
	c := workflowautomation.ExecutionCommand{ID: row.ID, OrganizationID: row.OrganizationID, WorkspaceID: row.WorkspaceID, TaskID: row.TaskID, ActorID: row.ActorID, StageVersion: row.StageVersion}
	if err = json.Unmarshal(row.Options, &c.Options); err != nil {
		return false, err
	}
	effects, err := deliver(ctx, c, commandExecutionHealth{queries: q, workspaceID: c.WorkspaceID, taskID: c.TaskID})
	if err != nil {
		return false, err
	}
	if err = q.SetWorkflowCommandState(ctx, sqlc.SetWorkflowCommandStateParams{ID: c.ID, State: effects.CommandState}); err != nil {
		return false, err
	}
	if err = q.SetQueuedStageState(ctx, sqlc.SetQueuedStageStateParams{WorkspaceID: c.WorkspaceID, ID: c.TaskID, StageVersion: c.StageVersion, StageWorkState: string(effects.WorkState)}); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

type ExecutionCommandPostgresRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func NewExecutionCommandPostgresRepository(database *PostgresDatabase) *ExecutionCommandPostgresRepository {
	return &ExecutionCommandPostgresRepository{pool: database.pool, queries: sqlc.New(database.pool)}
}

// commandExecutionHealth reads and locks the current run in the delivery transaction.
type commandExecutionHealth struct {
	queries             *sqlc.Queries
	workspaceID, taskID string
}

func (r commandExecutionHealth) TaskExecutionFailed(ctx context.Context) (bool, error) {
	failed, err := r.queries.TaskExecutionFailed(ctx, sqlc.TaskExecutionFailedParams{WorkspaceID: r.workspaceID, ID: r.taskID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return failed, err
}
