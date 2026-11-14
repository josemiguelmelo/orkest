package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	stage "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration"
	stageintegrationmodel "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration/model"
)

type WorkflowStageIntegrationPostgresRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func NewWorkflowStageIntegrationPostgresRepository(database *PostgresDatabase) *WorkflowStageIntegrationPostgresRepository {
	return &WorkflowStageIntegrationPostgresRepository{pool: database.pool, queries: sqlc.New(database.pool)}
}

var _ stage.WorkflowStageIntegrationRepository = (*WorkflowStageIntegrationPostgresRepository)(nil)

func integrationConnection(row sqlc.WorkflowStageIntegration) stage.StoredConnection {
	return stage.StoredConnection{Connection: stageintegrationmodel.Connection{ID: row.ID, WorkspaceID: row.WorkspaceID, Provider: row.Provider, Name: row.Name, Enabled: row.Enabled, CodingIntegrationID: row.CodingIntegrationID, RepositoryIDs: row.RepositoryIds, Config: row.Config, ConfiguredBy: row.ConfiguredBy, Version: row.Version, UpdatedAt: row.UpdatedAt.Time}, EncryptedSecret: row.EncryptedSecret}
}

func (r *WorkflowStageIntegrationPostgresRepository) ListConnections(ctx context.Context, workspaceID string) ([]stageintegrationmodel.Connection, error) {
	rows, err := r.queries.ListWorkflowStageIntegrations(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	out := make([]stageintegrationmodel.Connection, 0, len(rows))
	for _, row := range rows {
		out = append(out, integrationConnection(row).Connection)
	}
	return out, nil
}

func (r *WorkflowStageIntegrationPostgresRepository) GetConnection(ctx context.Context, workspaceID, id string) (stage.StoredConnection, error) {
	row, err := r.queries.GetWorkflowStageIntegration(ctx, sqlc.GetWorkflowStageIntegrationParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return stage.StoredConnection{}, mapError(err, problem.ErrNotFound, problem.ErrConflict)
	}
	return integrationConnection(row), nil
}

func (r *WorkflowStageIntegrationPostgresRepository) CallbackConnection(ctx context.Context, id string) (stage.StoredConnection, error) {
	// Connection IDs are globally unique. Workspace scope always comes from this
	// stored row; no callback body can select a workspace.
	row, err := r.queries.LocklessWorkflowStageCallbackIntegration(ctx, id)
	if err != nil {
		return stage.StoredConnection{}, mapError(err, problem.ErrNotFound, problem.ErrConflict)
	}
	return integrationConnection(row), nil
}

func validateIntegrationRepositories(ctx context.Context, q *sqlc.Queries, c stageintegrationmodel.Connection) error {
	rows, err := q.WorkflowStageIntegrationRepositories(ctx, sqlc.WorkflowStageIntegrationRepositoriesParams{WorkspaceID: c.WorkspaceID, IntegrationID: pgtype.Text{String: c.CodingIntegrationID, Valid: true}, RepositoryIds: c.RepositoryIDs})
	if err != nil {
		return err
	}
	if len(rows) == 0 || len(rows) != len(c.RepositoryIDs) {
		return problem.ErrInvalid
	}
	_, err = q.WorkflowStageIntegrationCodingProvider(ctx, sqlc.WorkflowStageIntegrationCodingProviderParams{WorkspaceID: c.WorkspaceID, ID: c.CodingIntegrationID})
	return mapError(err, problem.ErrInvalid, problem.ErrConflict)
}

func (r *WorkflowStageIntegrationPostgresRepository) CreateConnection(ctx context.Context, in stage.StoredConnection) (stageintegrationmodel.Connection, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return stageintegrationmodel.Connection{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	c := in.Connection
	if err = validateIntegrationRepositories(ctx, q, c); err != nil {
		return stageintegrationmodel.Connection{}, err
	}
	row, err := q.CreateWorkflowStageIntegration(ctx, sqlc.CreateWorkflowStageIntegrationParams{ID: c.ID, WorkspaceID: c.WorkspaceID, Provider: c.Provider, Name: c.Name, Enabled: c.Enabled, CodingIntegrationID: c.CodingIntegrationID, RepositoryIds: c.RepositoryIDs, Config: c.Config, EncryptedSecret: in.EncryptedSecret, ConfiguredBy: c.ConfiguredBy})
	if err != nil {
		return stageintegrationmodel.Connection{}, mapError(err, problem.ErrNotFound, problem.ErrConflict)
	}
	return integrationConnection(row).Connection, tx.Commit(ctx)
}

func (r *WorkflowStageIntegrationPostgresRepository) UpdateConnection(ctx context.Context, in stage.StoredConnection) (stageintegrationmodel.Connection, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return stageintegrationmodel.Connection{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	c := in.Connection
	old, err := q.LockWorkflowStageIntegration(ctx, c.ID)
	if err != nil {
		return stageintegrationmodel.Connection{}, mapError(err, problem.ErrNotFound, problem.ErrConflict)
	}
	if old.WorkspaceID != c.WorkspaceID {
		return stageintegrationmodel.Connection{}, problem.ErrNotFound
	}
	if old.Provider != c.Provider || old.CodingIntegrationID != c.CodingIntegrationID {
		return stageintegrationmodel.Connection{}, problem.ErrInvalid
	}
	if err = validateIntegrationRepositories(ctx, q, c); err != nil {
		return stageintegrationmodel.Connection{}, err
	}
	row, err := q.UpdateWorkflowStageIntegration(ctx, sqlc.UpdateWorkflowStageIntegrationParams{WorkspaceID: c.WorkspaceID, ID: c.ID, Version: c.Version, Name: c.Name, Enabled: c.Enabled, RepositoryIds: c.RepositoryIDs, Config: c.Config, EncryptedSecret: in.EncryptedSecret, ConfiguredBy: c.ConfiguredBy})
	if err != nil {
		return stageintegrationmodel.Connection{}, mapError(err, problem.ErrConflict, problem.ErrConflict)
	}
	return integrationConnection(row).Connection, tx.Commit(ctx)
}

func (r *WorkflowStageIntegrationPostgresRepository) DeleteConnection(ctx context.Context, workspaceID, id string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	old, err := q.LockWorkflowStageIntegration(ctx, id)
	if err != nil {
		return mapError(err, problem.ErrNotFound, problem.ErrConflict)
	}
	if old.WorkspaceID != workspaceID {
		return problem.ErrNotFound
	}
	exists, err := q.WorkflowStageIntegrationReferenced(ctx, sqlc.WorkflowStageIntegrationReferencedParams{WorkspaceID: workspaceID, ConnectionID: id})
	if err != nil {
		return err
	}
	if exists {
		return problem.ErrConflict
	}
	_, err = q.DeleteWorkflowStageIntegration(ctx, sqlc.DeleteWorkflowStageIntegrationParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func lockIntegrationConnection(ctx context.Context, q *sqlc.Queries, c stageintegrationmodel.Connection) error {
	current, err := q.LockWorkflowStageIntegration(ctx, c.ID)
	if err != nil {
		return mapError(err, problem.ErrNotFound, problem.ErrConflict)
	}
	if current.WorkspaceID != c.WorkspaceID || current.Version != c.Version || !current.Enabled {
		return problem.ErrConflict
	}
	return nil
}

func (r *WorkflowStageIntegrationPostgresRepository) ConnectionRepositoryIDs(ctx context.Context, c stageintegrationmodel.Connection) ([]string, error) {
	rows, err := r.queries.WorkflowStageIntegrationRepositories(ctx, sqlc.WorkflowStageIntegrationRepositoriesParams{WorkspaceID: c.WorkspaceID, IntegrationID: pgtype.Text{String: c.CodingIntegrationID, Valid: true}, RepositoryIds: c.RepositoryIDs})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || len(rows) != len(c.RepositoryIDs) {
		return nil, problem.ErrInvalid
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if !row.ProviderRepositoryID.Valid || row.ProviderRepositoryID.String == "" {
			return nil, problem.ErrInvalid
		}
		ids = append(ids, row.ProviderRepositoryID.String)
	}
	return ids, nil
}
