package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/josemiguelmelo/orkest/internal/core/ids"
	"github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/taskintegration"
)

type TaskIntegrationPostgresRepository struct{ queries *sqlc.Queries }

func NewTaskIntegrationPostgresRepository(database *PostgresDatabase) *TaskIntegrationPostgresRepository {
	return &TaskIntegrationPostgresRepository{queries: sqlc.New(database.pool)}
}

var _ taskintegration.Repository = (*TaskIntegrationPostgresRepository)(nil)

func (r *TaskIntegrationPostgresRepository) Create(ctx context.Context, s taskintegration.Source, secret string) (taskintegration.Source, error) {
	seconds, err := intervalSeconds(s.Interval)
	if err != nil {
		return taskintegration.Source{}, err
	}
	row, err := r.queries.CreateTaskIntegration(ctx, sqlc.CreateTaskIntegrationParams{ID: s.ID, WorkspaceID: s.WorkspaceID, Provider: s.Provider, Name: s.Name, Enabled: s.Enabled, BaseUrl: s.Config.BaseURL, ProjectUrl: s.Config.ProjectURL, Projects: s.Config.Projects, Labels: s.Labels, IntervalSeconds: seconds, EncryptedCredential: secret})
	return sourceFrom(row), err
}

func (r *TaskIntegrationPostgresRepository) Update(ctx context.Context, workspaceID, id string, s taskintegration.Source, secret string) (taskintegration.Source, error) {
	seconds, err := intervalSeconds(s.Interval)
	if err != nil {
		return taskintegration.Source{}, err
	}
	row, err := r.queries.UpdateTaskIntegration(ctx, sqlc.UpdateTaskIntegrationParams{WorkspaceID: workspaceID, ID: id, Provider: s.Provider, Name: s.Name, Enabled: s.Enabled, BaseUrl: s.Config.BaseURL, ProjectUrl: s.Config.ProjectURL, Projects: s.Config.Projects, Labels: s.Labels, IntervalSeconds: seconds, EncryptedCredential: secret})
	if err != nil {
		return taskintegration.Source{}, mapError(err, errors.ErrNotFound, errors.ErrConflict)
	}
	return sourceFrom(row), nil
}

func (r *TaskIntegrationPostgresRepository) Delete(ctx context.Context, workspaceID, id string) error {
	n, err := r.queries.DeleteTaskIntegration(ctx, sqlc.DeleteTaskIntegrationParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.ErrNotFound
	}
	return nil
}

func (r *TaskIntegrationPostgresRepository) Get(ctx context.Context, workspaceID, id string) (taskintegration.Source, string, error) {
	row, err := r.queries.GetTaskIntegration(ctx, sqlc.GetTaskIntegrationParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return taskintegration.Source{}, "", mapError(err, errors.ErrNotFound, errors.ErrConflict)
	}
	return sourceFrom(row), row.EncryptedCredential, nil
}

func (r *TaskIntegrationPostgresRepository) List(ctx context.Context, workspaceID string) ([]taskintegration.Source, error) {
	rows, err := r.queries.ListTaskIntegrations(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]taskintegration.Source, 0, len(rows))
	for _, row := range rows {
		out = append(out, sourceFrom(row))
	}
	return out, nil
}

func (r *TaskIntegrationPostgresRepository) Due(ctx context.Context, at time.Time) ([]taskintegration.Source, error) {
	rows, err := r.queries.DueTaskIntegrations(ctx, pgtype.Timestamptz{Time: at, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]taskintegration.Source, 0, len(rows))
	for _, row := range rows {
		out = append(out, sourceFrom(row))
	}
	return out, nil
}

func (r *TaskIntegrationPostgresRepository) RecordSync(ctx context.Context, id string, at time.Time, message string) error {
	return r.queries.RecordTaskIntegrationSync(ctx, sqlc.RecordTaskIntegrationSyncParams{ID: id, LastSyncedAt: pgtype.Timestamptz{Time: at, Valid: true}, LastError: message})
}

type ImportedTaskPostgresRepository struct{ queries *sqlc.Queries }

func NewImportedTaskPostgresRepository(database *PostgresDatabase) *ImportedTaskPostgresRepository {
	return &ImportedTaskPostgresRepository{queries: sqlc.New(database.pool)}
}

var _ taskintegration.TaskImporter = (*ImportedTaskPostgresRepository)(nil)

func (r *ImportedTaskPostgresRepository) UpsertImportedTask(ctx context.Context, workspaceID, integrationID, provider string, item taskintegration.RemoteTask) error {
	if item.ID == "" || item.Title == "" {
		return fmt.Errorf("source task is missing identity or title")
	}
	id, err := ids.NewUUID()
	if err != nil {
		return err
	}
	return r.queries.UpsertImportedTask(ctx, sqlc.UpsertImportedTaskParams{ID: id, WorkspaceID: workspaceID, Title: item.Title, Description: item.Description, SourceProvider: provider, SourceIntegrationID: pgtype.Text{String: integrationID, Valid: true}, SourceID: pgtype.Text{String: item.ID, Valid: true}, SourceUrl: pgtype.Text{String: item.URL, Valid: item.URL != ""}, Labels: item.Labels, Closed: item.Closed})
}

func (r *ImportedTaskPostgresRepository) RemoveMissingImportedTasks(ctx context.Context, workspaceID, integrationID string, sourceIDs []string) error {
	return r.queries.RemoveMissingImportedTasks(ctx, sqlc.RemoveMissingImportedTasksParams{WorkspaceID: workspaceID, SourceIntegrationID: pgtype.Text{String: integrationID, Valid: true}, SourceIds: sourceIDs})
}

func sourceFrom(row sqlc.TaskIntegration) taskintegration.Source {
	interval := time.Duration(row.IntervalSeconds) * time.Second
	s := taskintegration.Source{ID: row.ID, WorkspaceID: row.WorkspaceID, Provider: row.Provider, Name: row.Name, Enabled: row.Enabled, Config: taskintegration.Config{BaseURL: row.BaseUrl, ProjectURL: row.ProjectUrl, Projects: row.Projects}, Labels: row.Labels, Interval: interval.String(), LastError: row.LastError, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}
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
