package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/josemiguelmelo/orkest/internal/core/ids"
)

func TestSessionMigrations(t *testing.T) {
	dsn, err := connectionStringFromEnvironment()
	if err != nil {
		t.Skip("PostgreSQL connection settings required for fresh migration verification")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	id, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	schema := "test_sessions_" + strings.ReplaceAll(id, "-", "")
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := base.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := base.Exec(ctx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := runMigrations(ctx, pool); err != nil {
		t.Fatal("fresh migrations failed", err)
	}
	var constraints int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conrelid = 'agent_launches'::regclass AND conname = 'agent_launch_session_scope_fk'`).Scan(&constraints); err != nil || constraints != 1 {
		t.Fatal("session scope constraint missing", err)
	}
}
