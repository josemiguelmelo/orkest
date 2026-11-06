package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	workflowautomation "github.com/josemiguelmelo/orkest/internal/server/workflowautomation"

	"github.com/jackc/pgx/v5"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
)

func (r *ExecutionCommandPostgresRepository) ClaimExecutionCommand(ctx context.Context, token string, lease time.Duration) (workflowautomation.ExecutionClaim, bool, error) {
	row, err := r.queries.ClaimWorkflowCommand(ctx, sqlc.ClaimWorkflowCommandParams{ClaimToken: token, LeaseSeconds: lease.Seconds()})
	if errors.Is(err, pgx.ErrNoRows) {
		return workflowautomation.ExecutionClaim{}, false, nil
	}
	if err != nil {
		return workflowautomation.ExecutionClaim{}, false, fmt.Errorf("claim workflow command: %w", err)
	}
	command := workflowautomation.ExecutionCommand{ID: row.ID, OrganizationID: row.OrganizationID, WorkspaceID: row.WorkspaceID, TaskID: row.TaskID, ActorID: row.ActorID, StageVersion: row.StageVersion}
	if err = json.Unmarshal(row.Options, &command.Options); err != nil {
		return workflowautomation.ExecutionClaim{}, false, fmt.Errorf("decode workflow command: %w", err)
	}
	return workflowautomation.ExecutionClaim{Command: command, Token: row.ClaimToken, Attempt: row.ClaimAttempt}, true, nil
}

func (r *ExecutionCommandPostgresRepository) CompleteExecutionCommand(ctx context.Context, claim workflowautomation.ExecutionClaim, decide func(bool) workflowautomation.ExecutionDeliveryEffects) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	if _, err = q.LockWorkflowDeliveryClaim(ctx, sqlc.LockWorkflowDeliveryClaimParams{ID: claim.Command.ID, ClaimToken: claim.Token}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return workflowautomation.ErrStaleDeliveryClaim
		}
		return err
	}
	failed, err := (commandExecutionHealth{queries: q, workspaceID: claim.Command.WorkspaceID, taskID: claim.Command.TaskID}).TaskExecutionFailed(ctx)
	if err != nil {
		return err
	}
	effects := decide(failed)
	count, err := q.AcknowledgeWorkflowDeliveryClaim(ctx, sqlc.AcknowledgeWorkflowDeliveryClaimParams{ID: claim.Command.ID, ClaimToken: claim.Token, State: effects.CommandState, DeliveryError: effects.FailureReason})
	if err != nil {
		return err
	}
	if count != 1 {
		return workflowautomation.ErrStaleDeliveryClaim
	}
	if err = q.SetQueuedStageState(ctx, sqlc.SetQueuedStageStateParams{WorkspaceID: claim.Command.WorkspaceID, ID: claim.Command.TaskID, StageVersion: claim.Command.StageVersion, StageWorkState: string(effects.WorkState)}); err != nil {
		return err
	}
	return tx.Commit(ctx)
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
