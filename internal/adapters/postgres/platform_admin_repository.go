package postgres

import (
	"context"

	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/server/platformadmin"
)

type PlatformAdminPostgresRepository struct {
	queries *sqlc.Queries
}

func NewPlatformAdminPostgresRepository(database *PostgresDatabase) *PlatformAdminPostgresRepository {
	return &PlatformAdminPostgresRepository{queries: sqlc.New(database.pool)}
}

var _ platformadmin.OverviewRepository = (*PlatformAdminPostgresRepository)(nil)

func (r *PlatformAdminPostgresRepository) PlatformOverview(ctx context.Context) (platformadmin.Overview, error) {
	row, err := r.queries.PlatformAdminOverview(ctx)
	if err != nil {
		return platformadmin.Overview{}, err
	}
	return platformadmin.Overview{
		EnabledUsers: row.EnabledUsers, Organizations: row.Organizations,
		Workspaces: row.Workspaces, RunsLast30Days: row.RunsLast30Days,
	}, nil
}
