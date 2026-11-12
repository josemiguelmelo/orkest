package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	jobmodel "github.com/josemiguelmelo/orkest/internal/server/scheduledjob/model"
)

func (r *AgentLaunchPostgresRepository) IsJobRun(ctx context.Context, runID string) (bool, error) {
	return r.queries.IsScheduledJobRun(ctx, runID)
}

func (r *AgentLaunchPostgresRepository) MarkJobTimeout(ctx context.Context, id string) error {
	return r.queries.MarkScheduledJobTimeout(ctx, id)
}

func (r *AgentLaunchPostgresRepository) ReserveJobSessionRun(ctx context.Context, run jobmodel.Run, config orchestrationmodel.RunConfig) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	claim, err := q.LockScheduledJobClaim(ctx, sqlc.LockScheduledJobClaimParams{ID: run.ID, ClaimToken: run.ClaimToken})
	if errors.Is(err, pgx.ErrNoRows) {
		return modelerrors.ErrConflict
	}
	if err != nil {
		return err
	}
	if claim.CancelRequested || claim.RunID != config.RunID || claim.SessionID != config.TaskSessionID || claim.WorkspaceID != config.WorkspaceID || claim.ID != config.TaskID || claim.CommandID != config.InitialCommandID {
		return modelerrors.ErrConflict
	}
	if _, err = q.LockScheduledJobWorkspace(ctx, claim.WorkspaceID); err != nil {
		return err
	}
	admissible, err := q.ScheduledJobAdmissible(ctx, claim.ID)
	if err != nil {
		return err
	}
	if !admissible.Valid || !admissible.Bool {
		return modelerrors.ErrConflict
	}
	allowed, err := q.ScheduledJobCanMutate(ctx, sqlc.ScheduledJobCanMutateParams{ID: claim.WorkspaceID, UserID: claim.ActorID})
	if err != nil {
		return err
	}
	if !allowed {
		return modelerrors.ErrForbidden
	}
	available, err := q.ScheduledJobRepositoryIDs(ctx, claim.WorkspaceID)
	if err != nil {
		return err
	}
	present := map[string]bool{}
	for _, id := range available {
		present[id] = true
	}
	for _, id := range claim.RepositoryIds {
		if !present[id] {
			return modelerrors.ErrNotFound
		}
	}
	if _, err = q.GetPromptOutbox(ctx, sqlc.GetPromptOutboxParams{RunID: config.RunID, CommandID: config.InitialCommandID}); err == nil {
		return modelerrors.ErrConflict
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	snapshot, err := r.sessionSnapshot(config)
	if err != nil {
		return err
	}
	if err = q.InsertTaskAgentSession(ctx, sqlc.InsertTaskAgentSessionParams{ID: config.TaskSessionID, OrganizationID: config.OrganizationID, WorkspaceID: config.WorkspaceID, TaskID: config.TaskID, Configuration: snapshot}); err != nil {
		return err
	}
	if err = q.InsertAgentLaunch(ctx, sqlc.InsertAgentLaunchParams{RunID: config.RunID, OrganizationID: config.OrganizationID, WorkspaceID: config.WorkspaceID, TaskID: config.TaskID, TaskSessionID: pgtype.Text{String: config.TaskSessionID, Valid: true}, Status: "starting", ExecutionImage: config.ExecutionImage}); err != nil {
		return err
	}
	if err = q.InsertPromptOutbox(ctx, sqlc.InsertPromptOutboxParams{RunID: config.RunID, CommandID: config.InitialCommandID, Prompt: config.InitialPrompt}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
