package postgres

import (
	"context"

	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/workspaceenv"
)

type WorkspaceEnvironmentPostgresRepository struct{ queries *sqlc.Queries }

func NewWorkspaceEnvironmentPostgresRepository(database *PostgresDatabase) *WorkspaceEnvironmentPostgresRepository {
	return &WorkspaceEnvironmentPostgresRepository{queries: sqlc.New(database.pool)}
}

var _ workspaceenv.WorkspaceEnvironmentRepository = (*WorkspaceEnvironmentPostgresRepository)(nil)

func (r *WorkspaceEnvironmentPostgresRepository) List(ctx context.Context, workspaceID string) ([]workspaceenv.StoredValue, error) {
	rows, err := r.queries.ListWorkspaceEnvironment(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	result := make([]workspaceenv.StoredValue, 0, len(rows))
	for _, row := range rows {
		result = append(result, workspaceenv.StoredValue{Name: row.Name, Value: row.Value, Encrypted: row.Encrypted})
	}
	return result, nil
}

func (r *WorkspaceEnvironmentPostgresRepository) Resolve(ctx context.Context, workspaceID string) ([]workspaceenv.StoredValue, error) {
	rows, err := r.queries.ResolveWorkspaceEnvironment(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	result := make([]workspaceenv.StoredValue, 0, len(rows))
	for _, row := range rows {
		result = append(result, workspaceenv.StoredValue{Name: row.Name, Value: row.Value, Encrypted: row.Encrypted})
	}
	return result, nil
}

func (r *WorkspaceEnvironmentPostgresRepository) Put(ctx context.Context, workspaceID, name, value string, secret bool) error {
	return r.queries.PutWorkspaceEnvironment(ctx, sqlc.PutWorkspaceEnvironmentParams{WorkspaceID: workspaceID, Name: name, Value: value, IsSecret: secret})
}

func (r *WorkspaceEnvironmentPostgresRepository) Delete(ctx context.Context, workspaceID, name string) error {
	count, err := r.queries.DeleteWorkspaceEnvironment(ctx, sqlc.DeleteWorkspaceEnvironmentParams{WorkspaceID: workspaceID, Name: name})
	if err == nil && count == 0 {
		return modelerrors.ErrNotFound
	}
	return err
}
