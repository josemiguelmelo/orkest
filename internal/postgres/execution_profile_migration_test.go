package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/josemiguelmelo/orkest/db/migrations"
	"github.com/josemiguelmelo/orkest/internal/core/ids"
)

// TestExecutionProfileMigration isolates this migration from unrelated historical migrations.
func TestExecutionProfileMigration(t *testing.T) {
	dsn, err := connectionStringFromEnvironment()
	if err != nil {
		t.Skip("PostgreSQL connection settings required")
	}
	ctx := context.Background()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	base, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	id, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	schema := pgx.Identifier{"test_profiles_" + strings.ReplaceAll(id, "-", "")}.Sanitize()
	if _, err = base.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = base.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") }()
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	// Existing tables required by this additive migration, without unrelated product data.
	if _, err = pool.Exec(ctx, `CREATE TABLE workspaces(id text PRIMARY KEY); CREATE TABLE agents(id text PRIMARY KEY,workspace_id text NOT NULL REFERENCES workspaces(id)); CREATE TABLE agent_launches(run_id text PRIMARY KEY); INSERT INTO workspaces VALUES ('workspace');`); err != nil {
		t.Fatal(err)
	}
	source, err := migrations.FS.ReadFile("0025_execution_profiles.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, down, ok := strings.Cut(string(source), "-- +goose Down")
	if !ok {
		t.Fatal("missing rollback")
	}
	if _, err = pool.Exec(ctx, up); err != nil {
		t.Fatal(err)
	}
	testExecutionProfiles(t, &PostgresDatabase{pool: pool}, "workspace")
	if _, err = pool.Exec(ctx, down); err != nil {
		t.Fatal("rollback", err)
	}
}
