package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/josemiguelmelo/orkest/internal/task"

	stderrors "errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/josemiguelmelo/orkest/internal/core/ids"
	"github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/taskintegration"
)

type PostgresTaskIntegrationRepository struct{ queries *sqlc.Queries }

func NewPostgresTaskIntegrationRepository(database *PostgresDatabase) *PostgresTaskIntegrationRepository {
	return &PostgresTaskIntegrationRepository{queries: sqlc.New(database.pool)}
}

var _ taskintegration.TaskIntegrationRepository = (*PostgresTaskIntegrationRepository)(nil)

func (r *PostgresTaskIntegrationRepository) Create(ctx context.Context, s taskintegration.TaskIntegration, secret string) (taskintegration.TaskIntegration, error) {
	seconds, err := intervalSeconds(s.Interval)
	if err != nil {
		return taskintegration.TaskIntegration{}, err
	}
	row, err := r.queries.CreateTaskIntegration(ctx, sqlc.CreateTaskIntegrationParams{ID: s.ID, WorkspaceID: s.WorkspaceID, WorkflowID: s.WorkflowID, Provider: string(s.Provider), Name: s.Name, Enabled: s.Enabled, ProviderConfig: s.Config, Labels: s.Labels, IntervalSeconds: seconds, EncryptedCredential: secret})
	if err != nil {
		return taskintegration.TaskIntegration{}, mapError(err, errors.ErrNotFound, errors.ErrConflict)
	}
	return taskIntegrationFromRow(row), nil
}

func (r *PostgresTaskIntegrationRepository) Update(ctx context.Context, workspaceID, id string, s taskintegration.TaskIntegration, secret string) (taskintegration.TaskIntegration, error) {
	seconds, err := intervalSeconds(s.Interval)
	if err != nil {
		return taskintegration.TaskIntegration{}, err
	}
	row, err := r.queries.UpdateTaskIntegration(ctx, sqlc.UpdateTaskIntegrationParams{WorkspaceID: workspaceID, ID: id, WorkflowID: s.WorkflowID, Provider: string(s.Provider), Name: s.Name, Enabled: s.Enabled, ProviderConfig: s.Config, Labels: s.Labels, IntervalSeconds: seconds, EncryptedCredential: secret})
	if err != nil {
		return taskintegration.TaskIntegration{}, mapError(err, errors.ErrNotFound, errors.ErrConflict)
	}
	return taskIntegrationFromRow(row), nil
}

func (r *PostgresTaskIntegrationRepository) Delete(ctx context.Context, workspaceID, id string) error {
	n, err := r.queries.DeleteTaskIntegration(ctx, sqlc.DeleteTaskIntegrationParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.ErrNotFound
	}
	return nil
}

func (r *PostgresTaskIntegrationRepository) Get(ctx context.Context, workspaceID, id string) (taskintegration.TaskIntegration, string, error) {
	row, err := r.queries.GetTaskIntegration(ctx, sqlc.GetTaskIntegrationParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return taskintegration.TaskIntegration{}, "", mapError(err, errors.ErrNotFound, errors.ErrConflict)
	}
	return taskIntegrationFromRow(row), row.EncryptedCredential, nil
}

func (r *PostgresTaskIntegrationRepository) List(ctx context.Context, workspaceID string) ([]taskintegration.TaskIntegration, error) {
	rows, err := r.queries.ListTaskIntegrations(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]taskintegration.TaskIntegration, 0, len(rows))
	for _, row := range rows {
		out = append(out, taskIntegrationFromRow(row))
	}
	return out, nil
}

func (r *PostgresTaskIntegrationRepository) Due(ctx context.Context, at time.Time) ([]taskintegration.TaskIntegration, error) {
	rows, err := r.queries.DueTaskIntegrations(ctx, pgtype.Timestamptz{Time: at, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]taskintegration.TaskIntegration, 0, len(rows))
	for _, row := range rows {
		out = append(out, taskIntegrationFromRow(row))
	}
	return out, nil
}

func (r *PostgresTaskIntegrationRepository) RecordSync(ctx context.Context, id string, at time.Time, message string) error {
	return r.queries.RecordTaskIntegrationSync(ctx, sqlc.RecordTaskIntegrationSyncParams{ID: id, LastSyncedAt: pgtype.Timestamptz{Time: at, Valid: true}, LastError: message})
}

type PostgresImportedTaskRepository struct {
	queries *sqlc.Queries
	pool    *pgxpool.Pool
}

func NewPostgresImportedTaskRepository(database *PostgresDatabase) *PostgresImportedTaskRepository {
	return &PostgresImportedTaskRepository{queries: sqlc.New(database.pool), pool: database.pool}
}

var _ taskintegration.ImportedTaskRepository = (*PostgresImportedTaskRepository)(nil)

func (r *PostgresImportedTaskRepository) UpsertImportedTask(ctx context.Context, workspaceID, integrationID, workflowID, provider string, item taskintegration.DiscoveredTask, decide func(task.ImportedTaskSnapshot) (task.ImportedTaskEffects, error)) error {
	if decide == nil {
		return fmt.Errorf("imported task entry decider is required")
	}
	id, err := ids.NewUUID()
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	data, err := q.LockWorkflow(ctx, sqlc.LockWorkflowParams{WorkspaceID: workspaceID, ID: workflowID})
	if err != nil {
		return err
	}
	d, err := workflowDefinition(ctx, q, data)
	if err != nil {
		return err
	}
	lookup := sqlc.ImportedWorkflowTaskParams{WorkspaceID: workspaceID, SourceProvider: provider, SourceID: pgtype.Text{String: item.ID, Valid: true}, SourceIntegrationID: pgtype.Text{String: integrationID, Valid: true}}
	before, err := q.ImportedWorkflowTask(ctx, lookup)
	if err != nil && !stderrors.Is(err, pgx.ErrNoRows) {
		return err
	}
	snapshot := task.ImportedTaskSnapshot{Workflow: d, SourceID: item.ID, Title: item.Title, Exists: before.ID != "", SourceClosed: item.Closed, StageID: before.Status}
	if before.ID != "" {
		state, e := q.LockWorkflowTask(ctx, sqlc.LockWorkflowTaskParams{WorkspaceID: workspaceID, ID: before.ID})
		if e != nil {
			return e
		}
		snapshot.AssigneeAgentID = state.Assignee
	}
	decision, err := decide(snapshot)
	if err != nil {
		return err
	}

	err = q.UpsertImportedTask(ctx, sqlc.UpsertImportedTaskParams{ID: id, WorkspaceID: workspaceID, WorkflowID: workflowID, Title: item.Title, Description: item.Description, SourceProvider: provider, SourceIntegrationID: pgtype.Text{String: integrationID, Valid: true}, SourceID: pgtype.Text{String: item.ID, Valid: true}, SourceUrl: pgtype.Text{String: item.URL, Valid: item.URL != ""}, Labels: item.Labels, Status: decision.StageID})
	if err != nil {
		return err
	}
	after, err := q.ImportedWorkflowTask(ctx, lookup)
	if err != nil {
		return err
	}
	if decision.EmitStageEntry || decision.WorkState != "" {
		state, e := q.LockWorkflowTask(ctx, sqlc.LockWorkflowTaskParams{WorkspaceID: workspaceID, ID: after.ID})
		if e != nil {
			return e
		}
		if decision.WorkState != "" {
			if e = q.SetStageWorkState(ctx, sqlc.SetStageWorkStateParams{WorkspaceID: workspaceID, ID: after.ID, StageVersion: state.StageVersion, StageWorkState: string(decision.WorkState)}); e != nil {
				return e
			}
		}
		if decision.EmitStageEntry {
			if e = q.MoveWorkflowTask(ctx, sqlc.MoveWorkflowTaskParams{WorkspaceID: workspaceID, ID: after.ID, StageVersion: state.StageVersion, Status: decision.StageID, Column5: decision.AssigneeAgentID}); e != nil {
				return e
			}
			if e = enqueueStageEntry(ctx, q, workspaceID, after.ID, d.ConfiguredBy, state.StageVersion, ""); e != nil {
				return e
			}
		}
	}

	return tx.Commit(ctx)
}

func (r *PostgresImportedTaskRepository) RemoveMissingImportedTasks(ctx context.Context, workspaceID, integrationID string, sourceIDs []string) error {
	return r.queries.RemoveMissingImportedTasks(ctx, sqlc.RemoveMissingImportedTasksParams{WorkspaceID: workspaceID, SourceIntegrationID: pgtype.Text{String: integrationID, Valid: true}, SourceIds: sourceIDs})
}

func taskIntegrationFromRow(row sqlc.TaskIntegration) taskintegration.TaskIntegration {
	interval := time.Duration(row.IntervalSeconds) * time.Second
	s := taskintegration.TaskIntegration{ID: row.ID, WorkspaceID: row.WorkspaceID, WorkflowID: row.WorkflowID, Provider: taskintegration.TaskIntegrationSourceProvider(row.Provider), Name: row.Name, Enabled: row.Enabled, Config: json.RawMessage(row.ProviderConfig), Labels: row.Labels, Interval: interval.String(), LastError: row.LastError, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}
	if row.LastSyncedAt.Valid {
		lastSynced := row.LastSyncedAt.Time
		s.LastSynced = &lastSynced
	}
	return s
}

func intervalSeconds(value string) (int32, error) {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, err
	}
	return int32(duration.Seconds()), nil
}
