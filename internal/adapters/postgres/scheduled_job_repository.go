package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	"github.com/josemiguelmelo/orkest/internal/server/scheduledjob"
	jobmodel "github.com/josemiguelmelo/orkest/internal/server/scheduledjob/model"
)

type ScheduledJobPostgresRepository struct {
	database *PostgresDatabase
	queries  *sqlc.Queries
}

func NewScheduledJobPostgresRepository(db *PostgresDatabase) *ScheduledJobPostgresRepository {
	return &ScheduledJobPostgresRepository{database: db, queries: sqlc.New(db.pool)}
}

func jobFromRow(row sqlc.ScheduledJob) (jobmodel.Job, error) {
	j := jobmodel.Job{ID: row.ID, WorkspaceID: row.WorkspaceID, Revision: row.Revision, AuthorizedBy: row.AuthorizedBy, NextDueAt: row.NextDueAt.Time, CreatedAt: row.CreatedAt.Time}
	err := json.Unmarshal(row.Definition, &j.Input)
	j.Enabled = row.Enabled
	return j, err
}

func jobRunFromRow(row sqlc.ScheduledJobRun, org string) (jobmodel.Run, error) {
	r := jobmodel.Run{ID: row.ID, JobID: row.JobID, WorkspaceID: row.WorkspaceID, OrganizationID: org, ActorID: row.ActorID, Revision: row.Revision, RepositoryIDs: row.RepositoryIds, ScheduledFor: row.ScheduledFor.Time, State: row.State, Reason: row.Reason, RunID: row.RunID, CommandID: row.CommandID, CreatedAt: row.CreatedAt.Time, ClaimToken: row.ClaimToken, ClaimAttempt: row.ClaimAttempt, CancelRequested: row.CancelRequested}
	err := json.Unmarshal(row.Definition, &r.Input)
	return r, err
}

func (r *ScheduledJobPostgresRepository) List(ctx context.Context, org, workspace string) ([]jobmodel.Job, error) {
	rows, err := r.queries.ListScheduledJobs(ctx, sqlc.ListScheduledJobsParams{OrganizationID: org, WorkspaceID: workspace})
	if err != nil {
		return nil, err
	}
	out := make([]jobmodel.Job, 0, len(rows))
	for _, row := range rows {
		j, e := jobFromRow(row)
		if e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	return out, nil
}

func (r *ScheduledJobPostgresRepository) Get(ctx context.Context, org, workspace, id string) (jobmodel.Job, error) {
	row, err := r.queries.GetScheduledJob(ctx, sqlc.GetScheduledJobParams{OrganizationID: org, WorkspaceID: workspace, ID: id})
	if err != nil {
		return jobmodel.Job{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	return jobFromRow(row)
}

func (r *ScheduledJobPostgresRepository) Save(ctx context.Context, user, org, workspace, id string, revision int64, in jobmodel.Input, next time.Time) (jobmodel.Job, error) {
	data, err := json.Marshal(in)
	if err != nil {
		return jobmodel.Job{}, err
	}
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return jobmodel.Job{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	validAgent, err := q.ScheduledJobAgentExists(ctx, sqlc.ScheduledJobAgentExistsParams{WorkspaceID: workspace, ID: in.AgentID})
	if err != nil {
		return jobmodel.Job{}, err
	}
	if !validAgent {
		return jobmodel.Job{}, modelerrors.ErrInvalid
	}
	available, err := q.ScheduledJobRepositoryIDs(ctx, workspace)
	if err != nil {
		return jobmodel.Job{}, err
	}
	registered := map[string]bool{}
	for _, id := range available {
		registered[id] = true
	}
	for _, id := range in.RepositoryIDs {
		if !registered[id] {
			return jobmodel.Job{}, modelerrors.ErrInvalid
		}
	}
	if revision == 0 {
		n, e := q.InsertScheduledJob(ctx, sqlc.InsertScheduledJobParams{ID: id, ID_2: workspace, OrganizationID: org, AuthorizedBy: user, Definition: data, Enabled: in.Enabled, NextDueAt: pgtype.Timestamptz{Time: next, Valid: true}})
		if e != nil {
			return jobmodel.Job{}, e
		}
		if n == 0 {
			prior, e := q.GetScheduledJobRevision(ctx, sqlc.GetScheduledJobRevisionParams{WorkspaceID: workspace, JobID: id, Revision: 1})
			if e != nil {
				return jobmodel.Job{}, modelerrors.ErrConflict
			}
			var original jobmodel.Input
			if e = json.Unmarshal(prior.Definition, &original); e != nil {
				return jobmodel.Job{}, e
			}
			if prior.AuthorizedBy != user || !reflect.DeepEqual(original, in) {
				return jobmodel.Job{}, modelerrors.ErrConflict
			}
			if e = tx.Commit(ctx); e != nil {
				return jobmodel.Job{}, e
			}
			return r.Get(ctx, org, workspace, id)
		}
		revision = 1
	} else {
		if _, err = q.LockScheduledJob(ctx, sqlc.LockScheduledJobParams{OrganizationID: org, WorkspaceID: workspace, ID: id}); err != nil {
			return jobmodel.Job{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
		}
		n, e := q.UpdateScheduledJob(ctx, sqlc.UpdateScheduledJobParams{WorkspaceID: workspace, ID: id, AuthorizedBy: user, Definition: data, Enabled: in.Enabled, NextDueAt: pgtype.Timestamptz{Time: next, Valid: true}, Revision: revision})
		if e != nil {
			return jobmodel.Job{}, e
		}
		if n != 1 {
			return jobmodel.Job{}, modelerrors.ErrConflict
		}
		revision++
	}
	if err = q.InsertScheduledJobRevision(ctx, sqlc.InsertScheduledJobRevisionParams{WorkspaceID: workspace, JobID: id, Revision: revision, AuthorizedBy: user, Definition: data}); err != nil {
		return jobmodel.Job{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return jobmodel.Job{}, err
	}
	return r.Get(ctx, org, workspace, id)
}

func (r *ScheduledJobPostgresRepository) Runs(ctx context.Context, org, workspace, job string, limit, offset int) ([]jobmodel.Run, error) {
	rows, err := r.queries.ListScheduledJobRuns(ctx, sqlc.ListScheduledJobRunsParams{OrganizationID: org, WorkspaceID: workspace, JobID: job, Limit: int32(limit), Offset: int32(offset)})
	if err != nil {
		return nil, err
	}
	out := make([]jobmodel.Run, 0, len(rows))
	for _, row := range rows {
		v, e := jobRunFromRow(row.ScheduledJobRun, row.OrganizationID)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}

func (r *ScheduledJobPostgresRepository) Run(ctx context.Context, org, workspace, job, id string) (jobmodel.Run, error) {
	row, err := r.queries.GetScheduledJobRun(ctx, sqlc.GetScheduledJobRunParams{OrganizationID: org, WorkspaceID: workspace, JobID: job, ID: id})
	if err != nil {
		return jobmodel.Run{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	return jobRunFromRow(row.ScheduledJobRun, row.OrganizationID)
}

func (r *ScheduledJobPostgresRepository) Enqueue(ctx context.Context, user, org, workspace, job, key string) (jobmodel.Run, error) {
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return jobmodel.Run{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	row, err := q.LockScheduledJob(ctx, sqlc.LockScheduledJobParams{OrganizationID: org, WorkspaceID: workspace, ID: job})
	if err != nil {
		return jobmodel.Run{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	previous, err := q.FindScheduledJobTrigger(ctx, sqlc.FindScheduledJobTriggerParams{OrganizationID: org, WorkspaceID: workspace, JobID: job, TriggerKey: "manual:" + key})
	if err == nil {
		if previous.ScheduledJobRun.ActorID != user {
			return jobmodel.Run{}, modelerrors.ErrConflict
		}
		return jobRunFromRow(previous.ScheduledJobRun, org)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return jobmodel.Run{}, err
	}
	row.AuthorizedBy = user
	run, err := r.insertOccurrence(ctx, q, row, org, time.Now().UTC(), "manual:"+key, false)
	if err != nil {
		return jobmodel.Run{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return jobmodel.Run{}, err
	}
	return run, nil
}

func (r *ScheduledJobPostgresRepository) Materialize(ctx context.Context) (bool, error) {
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	row, err := q.PendingScheduledJob(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	job, err := jobFromRow(row.ScheduledJob)
	if err != nil {
		return true, err
	}
	latest, next, err := scheduledjob.Due(job.Schedule, job.NextDueAt, row.DatabaseNow.Time)
	if err != nil {
		return true, err
	}
	_, err = r.insertOccurrence(ctx, q, row.ScheduledJob, row.OrganizationID, latest, "scheduled:"+latest.UTC().Format(time.RFC3339), row.DatabaseNow.Time.Sub(latest) > time.Hour)
	if err != nil {
		return true, err
	}
	if err = q.AdvanceScheduledJob(ctx, sqlc.AdvanceScheduledJobParams{WorkspaceID: job.WorkspaceID, ID: job.ID, NextDueAt: pgtype.Timestamptz{Time: next, Valid: true}}); err != nil {
		return true, err
	}
	return true, tx.Commit(ctx)
}

func (r *ScheduledJobPostgresRepository) insertOccurrence(ctx context.Context, q *sqlc.Queries, row sqlc.ScheduledJob, org string, at time.Time, key string, missed bool) (jobmodel.Run, error) {
	job, err := jobFromRow(row)
	if err != nil {
		return jobmodel.Run{}, err
	}
	identity, err := ids.NewUUID()
	if err != nil {
		return jobmodel.Run{}, err
	}
	occupied, err := q.ScheduledJobOccupied(ctx, sqlc.ScheduledJobOccupiedParams{WorkspaceID: row.WorkspaceID, JobID: row.ID})
	if err != nil {
		return jobmodel.Run{}, err
	}
	allowed, err := q.ScheduledJobCanMutate(ctx, sqlc.ScheduledJobCanMutateParams{ID: row.WorkspaceID, UserID: row.AuthorizedBy})
	if err != nil {
		return jobmodel.Run{}, err
	}
	available, err := q.ScheduledJobRepositoryIDs(ctx, row.WorkspaceID)
	if err != nil {
		return jobmodel.Run{}, err
	}
	selected := []string{}
	switch job.RepositoryScope {
	case "all":
		selected = available
	case "selected":
		selected = job.RepositoryIDs
	}
	if selected == nil {
		selected = []string{}
	}
	valid := map[string]bool{}
	for _, id := range available {
		valid[id] = true
	}
	resourcesValid := job.RepositoryScope == "none" || len(selected) > 0
	for _, id := range selected {
		if !valid[id] {
			resourcesValid = false
		}
	}
	state, reason := scheduledjob.OccurrenceState(occupied, allowed, resourcesValid, missed)
	if !allowed {
		if err = q.DisableScheduledJob(ctx, sqlc.DisableScheduledJobParams{WorkspaceID: row.WorkspaceID, ID: row.ID}); err != nil {
			return jobmodel.Run{}, err
		}
	}
	if err = q.InsertScheduledJobRun(ctx, sqlc.InsertScheduledJobRunParams{ID: identity, WorkspaceID: row.WorkspaceID, JobID: row.ID, Revision: row.Revision, ActorID: row.AuthorizedBy, Definition: row.Definition, RepositoryIds: selected, ScheduledFor: pgtype.Timestamptz{Time: at, Valid: true}, TriggerKey: key, State: state, Reason: reason, RunID: identity + ":run", SessionID: identity + ":session", CommandID: identity + ":start"}); err != nil {
		return jobmodel.Run{}, err
	}
	record, err := q.GetScheduledJobRun(ctx, sqlc.GetScheduledJobRunParams{OrganizationID: org, WorkspaceID: row.WorkspaceID, JobID: row.ID, ID: identity})
	if err != nil {
		return jobmodel.Run{}, err
	}
	return jobRunFromRow(record.ScheduledJobRun, org)
}

func (r *ScheduledJobPostgresRepository) Claim(ctx context.Context, token string) (jobmodel.Run, bool, error) {
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return jobmodel.Run{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	row, err := q.PendingScheduledJobRun(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return jobmodel.Run{}, false, nil
	}
	if err != nil {
		return jobmodel.Run{}, false, err
	}
	v := row.ScheduledJobRun
	if v.State == "pending" && row.DatabaseNow.Time.Sub(v.CreatedAt.Time) <= time.Hour {
		if _, err = q.LockScheduledJobWorkspace(ctx, v.WorkspaceID); err != nil {
			return jobmodel.Run{}, false, err
		}
		busy, e := q.ScheduledWorkspaceOccupied(ctx, sqlc.ScheduledWorkspaceOccupiedParams{WorkspaceID: v.WorkspaceID, ID: v.ID})
		if e != nil {
			return jobmodel.Run{}, false, e
		}
		if busy {
			return jobmodel.Run{}, false, nil
		}
	}
	if err = q.ClaimScheduledJobRun(ctx, sqlc.ClaimScheduledJobRunParams{ID: v.ID, ClaimToken: token}); err != nil {
		return jobmodel.Run{}, false, err
	}
	v.State, v.ClaimToken, v.ClaimAttempt = "starting", token, v.ClaimAttempt+1
	result, err := jobRunFromRow(v, row.OrganizationID)
	result.DeliveryExpired = row.DatabaseNow.Time.Sub(v.CreatedAt.Time) > time.Hour
	if err != nil {
		return jobmodel.Run{}, false, err
	}
	return result, true, tx.Commit(ctx)
}

func (r *ScheduledJobPostgresRepository) FinishClaim(ctx context.Context, run jobmodel.Run, state, reason string) error {
	_, err := r.queries.FinishScheduledJobClaim(ctx, sqlc.FinishScheduledJobClaimParams{ID: run.ID, ClaimToken: run.ClaimToken, State: state, Reason: reason})
	return err
}

func (r *ScheduledJobPostgresRepository) CancelPending(ctx context.Context, org, workspace, job, id string) (bool, error) {
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	// Lock before checking acceptance: a waiting cancellation must observe a
	// reservation committed by the previous owner of this row lock.
	if _, err = q.LockScopedScheduledJobRun(ctx, sqlc.LockScopedScheduledJobRunParams{OrganizationID: org, WorkspaceID: workspace, JobID: job, ID: id}); err != nil {
		return false, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	current, err := q.GetScheduledJobRun(ctx, sqlc.GetScheduledJobRunParams{OrganizationID: org, WorkspaceID: workspace, JobID: job, ID: id})
	if err != nil {
		return false, err
	}
	switch current.ScheduledJobRun.State {
	case "cancelled":
		return true, tx.Commit(ctx)
	case "pending", "starting", "running", "delivery_uncertain":
	default:
		return false, modelerrors.ErrConflict
	}
	if _, err = q.CancelPendingScheduledJobRun(ctx, sqlc.CancelPendingScheduledJobRunParams{WorkspaceID: workspace, JobID: job, ID: id}); err != nil {
		return false, err
	}
	row, err := q.GetScheduledJobRun(ctx, sqlc.GetScheduledJobRunParams{OrganizationID: org, WorkspaceID: workspace, JobID: job, ID: id})
	if err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return row.ScheduledJobRun.State == "cancelled", nil
}

func (r *ScheduledJobPostgresRepository) Expired(ctx context.Context) ([]jobmodel.Run, error) {
	rows, err := r.queries.ExpiredScheduledJobRuns(ctx)
	if err != nil {
		return nil, err
	}
	out := []jobmodel.Run{}
	for _, row := range rows {
		run, e := jobRunFromRow(row.ScheduledJobRun, row.OrganizationID)
		if e != nil {
			return nil, e
		}
		out = append(out, run)
	}
	return out, nil
}
