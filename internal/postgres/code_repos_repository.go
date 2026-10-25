package postgres

import (
	"context"

	"github.com/josemiguelmelo/orkest/internal/coderepository"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
)

type CodeReposPostgresRepository struct{ queries *sqlc.Queries }

func NewCodeReposPostgresRepository(database *PostgresDatabase) *CodeReposPostgresRepository {
	return &CodeReposPostgresRepository{queries: sqlc.New(database.pool)}
}

var _ coderepository.CodeReposRepository = (*CodeReposPostgresRepository)(nil)

func (r *CodeReposPostgresRepository) CodeRepositories(ctx context.Context, orgID, workspaceID string) ([]coderepository.CodeRepository, error) {
	rows, err := r.queries.ListCodeRepositories(ctx, sqlc.ListCodeRepositoriesParams{OrganizationID: orgID, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	out := make([]coderepository.CodeRepository, 0, len(rows))
	for _, row := range rows {
		out = append(out, coderepository.CodeRepository{ID: row.ID, WorkspaceID: row.WorkspaceID, ProviderName: row.ProviderName, URL: row.Url})
	}
	return out, nil
}

func (r *CodeReposPostgresRepository) CodeRepository(ctx context.Context, orgID, workspaceID, id string) (coderepository.CodeRepository, error) {
	row, err := r.queries.GetCodeRepository(ctx, sqlc.GetCodeRepositoryParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id})
	return coderepository.CodeRepository{ID: row.ID, WorkspaceID: row.WorkspaceID, ProviderName: row.ProviderName, URL: row.Url}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (r *CodeReposPostgresRepository) CreateCodeRepository(ctx context.Context, id, workspaceID, providerName, repositoryURL string) (coderepository.CodeRepository, error) {
	row, err := r.queries.InsertCodeRepository(ctx, sqlc.InsertCodeRepositoryParams{ID: id, ID_2: workspaceID, ProviderName: providerName, Url: repositoryURL})
	return coderepository.CodeRepository{ID: row.ID, WorkspaceID: row.WorkspaceID, ProviderName: row.ProviderName, URL: row.Url}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (r *CodeReposPostgresRepository) DeleteCodeRepository(ctx context.Context, orgID, workspaceID, id string) error {
	count, err := r.queries.DeleteCodeRepository(ctx, sqlc.DeleteCodeRepositoryParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id})
	if err == nil && count == 0 {
		return modelerrors.ErrNotFound
	}
	return err
}
