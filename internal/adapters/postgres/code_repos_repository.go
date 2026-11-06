package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/coderepository"
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
		out = append(out, mapCodeRepository(row.ID, row.WorkspaceID, row.ProviderName, row.Url, row.IntegrationID, row.ProviderRepositoryID, row.DefaultBranch, row.DisplayName, row.IsPrivate))
	}
	return out, nil
}

func (r *CodeReposPostgresRepository) CodeRepository(ctx context.Context, orgID, workspaceID, id string) (coderepository.CodeRepository, error) {
	row, err := r.queries.GetCodeRepository(ctx, sqlc.GetCodeRepositoryParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id})
	return mapCodeRepository(row.ID, row.WorkspaceID, row.ProviderName, row.Url, row.IntegrationID, row.ProviderRepositoryID, row.DefaultBranch, row.DisplayName, row.IsPrivate), mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (r *CodeReposPostgresRepository) CreateIntegratedCodeRepository(ctx context.Context, item coderepository.CodeRepository) (coderepository.CodeRepository, error) {
	row, err := r.queries.InsertIntegratedCodeRepository(ctx, sqlc.InsertIntegratedCodeRepositoryParams{ID: item.ID, WorkspaceID: item.WorkspaceID, ProviderName: item.ProviderName, Url: item.URL, IntegrationID: pgtype.Text{String: item.IntegrationID, Valid: true}, ProviderRepositoryID: pgtype.Text{String: item.ProviderRepositoryID, Valid: true}, DefaultBranch: item.DefaultBranch, DisplayName: item.Name, IsPrivate: item.Private})
	return mapCodeRepository(row.ID, row.WorkspaceID, row.ProviderName, row.Url, row.IntegrationID, row.ProviderRepositoryID, row.DefaultBranch, row.DisplayName, row.IsPrivate), mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
}

func mapCodeRepository(id, workspaceID, providerName, url string, integrationID, providerRepositoryID pgtype.Text, defaultBranch, name string, isPrivate bool) coderepository.CodeRepository {
	item := coderepository.CodeRepository{ID: id, WorkspaceID: workspaceID, ProviderName: providerName, URL: url, DefaultBranch: defaultBranch, Name: name, Private: isPrivate}
	if integrationID.Valid {
		item.IntegrationID = integrationID.String
	}
	if providerRepositoryID.Valid {
		item.ProviderRepositoryID = providerRepositoryID.String
	}
	return item
}

func (r *CodeReposPostgresRepository) DeleteCodeRepository(ctx context.Context, orgID, workspaceID, id string) error {
	count, err := r.queries.DeleteCodeRepository(ctx, sqlc.DeleteCodeRepositoryParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return modelerrors.ErrConflict
		}
		return mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	if count == 0 {
		return modelerrors.ErrNotFound
	}
	return nil
}

var _ coderepository.CodingIntegrationRepository = (*CodeReposPostgresRepository)(nil)

func (r *CodeReposPostgresRepository) ListCodingIntegrations(ctx context.Context, workspaceID string) ([]coderepository.CodingIntegration, error) {
	rows, err := r.queries.ListCodingIntegrations(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]coderepository.CodingIntegration, 0, len(rows))
	for _, row := range rows {
		out = append(out, coderepository.CodingIntegration{ID: row.ID, WorkspaceID: row.WorkspaceID, Name: row.Name, ProviderName: row.ProviderName, ProviderConfig: json.RawMessage(row.ProviderConfig), UpdatedAt: row.UpdatedAt.Time.Format(time.RFC3339)})
	}
	return out, nil
}

func (r *CodeReposPostgresRepository) GetCodingIntegration(ctx context.Context, workspaceID, id string) (coderepository.CodingIntegration, string, error) {
	row, err := r.queries.GetCodingIntegration(ctx, sqlc.GetCodingIntegrationParams{WorkspaceID: workspaceID, ID: id})
	item := coderepository.CodingIntegration{ID: row.ID, WorkspaceID: row.WorkspaceID, Name: row.Name, ProviderName: row.ProviderName, ProviderConfig: json.RawMessage(row.ProviderConfig), UpdatedAt: row.UpdatedAt.Time.Format(time.RFC3339)}
	return item, row.EncryptedCredential, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (r *CodeReposPostgresRepository) CreateCodingIntegration(ctx context.Context, item coderepository.CodingIntegration, credential string) (coderepository.CodingIntegration, error) {
	row, err := r.queries.InsertCodingIntegration(ctx, sqlc.InsertCodingIntegrationParams{ID: item.ID, WorkspaceID: item.WorkspaceID, Name: item.Name, ProviderName: item.ProviderName, ProviderConfig: item.ProviderConfig, EncryptedCredential: credential})
	item = coderepository.CodingIntegration{ID: row.ID, WorkspaceID: row.WorkspaceID, Name: row.Name, ProviderName: row.ProviderName, ProviderConfig: json.RawMessage(row.ProviderConfig), UpdatedAt: row.UpdatedAt.Time.Format(time.RFC3339)}
	return item, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (r *CodeReposPostgresRepository) UpdateCodingIntegration(ctx context.Context, item coderepository.CodingIntegration, credential string) error {
	count, err := r.queries.UpdateCodingIntegration(ctx, sqlc.UpdateCodingIntegrationParams{WorkspaceID: item.WorkspaceID, ID: item.ID, Name: item.Name, ProviderConfig: item.ProviderConfig, EncryptedCredential: credential})
	if err == nil && count == 0 {
		return modelerrors.ErrNotFound
	}
	return mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (r *CodeReposPostgresRepository) DeleteCodingIntegration(ctx context.Context, workspaceID, id string) error {
	count, err := r.queries.DeleteCodingIntegration(ctx, sqlc.DeleteCodingIntegrationParams{WorkspaceID: workspaceID, ID: id})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23503" || pgErr.Code == "23001") {
		return modelerrors.ErrConflict
	}
	if err == nil && count == 0 {
		return modelerrors.ErrNotFound
	}
	return mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}
