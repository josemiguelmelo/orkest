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

func TestCodexRuntimeMigration(t *testing.T) {
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
	schema := pgx.Identifier{"test_codex_runtime_" + strings.ReplaceAll(id, "-", "")}.Sanitize()
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
	if _, err = pool.Exec(ctx, `CREATE TABLE agents (
		id text PRIMARY KEY,
		runtime text NOT NULL CONSTRAINT agents_runtime_check CHECK (runtime IN ('opencode', 'claude-code'))
	)`); err != nil {
		t.Fatal(err)
	}
	source, err := migrations.FS.ReadFile("0027_codex_runtime.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, down, ok := strings.Cut(string(source), "-- +goose Down")
	if !ok {
		t.Fatal("missing rollback")
	}
	if _, err = pool.Exec(ctx, up); err != nil {
		t.Fatal("apply migration", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO agents (id, runtime) VALUES ('codex-agent', 'codex')`); err != nil {
		t.Fatal("Codex runtime rejected after migration", err)
	}
	if _, err = pool.Exec(ctx, down); err == nil || !strings.Contains(err.Error(), "cannot remove codex runtime") {
		t.Fatal("rollback should refuse while Codex agents exist", err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM agents WHERE runtime = 'codex'`); err != nil {
		t.Fatal("remove Codex fixture", err)
	}
	if _, err = pool.Exec(ctx, down); err != nil {
		t.Fatal("rollback migration", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO agents (id, runtime) VALUES ('codex-agent', 'codex')`); err == nil {
		t.Fatal("Codex runtime remained allowed after rollback")
	}
}
