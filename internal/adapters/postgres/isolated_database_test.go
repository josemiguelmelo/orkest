package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
)

// Repository fixtures require the current migration chain, independent of the
// developer database's schema version, and must not leave product data behind.
func isolatedRepositoryDatabase(t *testing.T, ctx context.Context, dsn string) (*PostgresDatabase, error) {
	t.Helper()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	id, err := ids.NewUUID()
	if err != nil {
		base.Close()
		return nil, err
	}
	schema := pgx.Identifier{"test_repositories_" + strings.ReplaceAll(id, "-", "")}.Sanitize()
	if _, err = base.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		base.Close()
		return nil, err
	}
	t.Cleanup(func() {
		if _, err := base.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
		base.Close()
	})
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	config.ConnConfig.RuntimeParams["search_path"] = strings.Trim(schema, "\"")
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	if err = runMigrations(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return &PostgresDatabase{pool: pool, ConnectionInfo: connectionInfo(config.ConnConfig)}, nil
}
