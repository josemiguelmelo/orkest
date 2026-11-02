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
	t.Run("workflow migration round trip", func(t *testing.T) { testWorkflowMigrationRoundTrip(t, ctx, pool) })
}

func testWorkflowMigrationRoundTrip(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `INSERT INTO organizations(id,name) VALUES('migration-org','Migration'); INSERT INTO workspaces(id,organization_id,name) VALUES('migration-workspace','migration-org','Migration'); INSERT INTO tasks(id,workspace_id,workflow_id,title,status) VALUES('migration-task','migration-workspace','migration-workspace','Migration','todo');`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "SET CONSTRAINTS ALL IMMEDIATE"); err != nil {
		t.Fatal(err)
	}

	if _, err = tx.Exec(ctx, `INSERT INTO workflow_stages(workspace_id,workflow_id,id,position,name) VALUES('migration-workspace','migration-workspace','custom',4,'Custom'); UPDATE tasks SET status='custom' WHERE id='migration-task'; INSERT INTO task_integrations(id,workspace_id,workflow_id,provider,name,encrypted_credential) VALUES('migration-integration','migration-workspace','migration-workspace','github_cloud','Migration','test'); SET CONSTRAINTS ALL IMMEDIATE;`); err != nil {
		t.Fatal(err)
	}
	automationSource, err := migrations.FS.ReadFile("0036_workflow_automation.sql")
	if err != nil {
		t.Fatal(err)
	}
	automationUp, automationDown, ok := strings.Cut(string(automationSource), "-- +goose Down")
	if !ok {
		t.Fatal("missing automation rollback")
	}
	if _, err = tx.Exec(ctx, automationDown); err != nil {
		t.Fatal("rollback automation", err)
	}
	source, err := migrations.FS.ReadFile("0035_workspace_workflows.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, down, ok := strings.Cut(string(source), "-- +goose Down")
	if !ok {
		t.Fatal("missing rollback")
	}
	if _, err = tx.Exec(ctx, down); err != nil {
		t.Fatal("rollback final workflow migration", err)
	}
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM tasks WHERE id='migration-task'`).Scan(&status); err != nil || status != "todo" {
		t.Fatal("custom stage was not mapped to todo", status, err)
	}
	if _, err = tx.Exec(ctx, up); err != nil {
		t.Fatal("reapply final workflow migration", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workflow_commands(id,workspace_id,task_id,stage_version,actor_id,options) VALUES('migration-outcome','migration-workspace','migration-task',1,'actor','{"action":"run_outcome","runId":"migration-run","success":true}');`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, automationUp); err != nil {
		t.Fatal("reapply automation", err)
	}
	var eventPreserved bool
	if err = tx.QueryRow(ctx, `SELECT event_type='run_succeeded' AND run_id='migration-run' AND processed_at IS NULL FROM workflow_events WHERE id='migration-outcome'`).Scan(&eventPreserved); err != nil || !eventPreserved {
		t.Fatal("pending outcome lost during upgrade", err)
	}
	var preserved bool
	if err = tx.QueryRow(ctx, `SELECT t.workflow_id='migration-workspace' AND t.status='todo' AND t.stage_version=1 AND i.workflow_id=t.workflow_id AND s.name='Todo' FROM tasks t JOIN task_integrations i ON i.workspace_id=t.workspace_id JOIN workflow_stages s ON s.workspace_id=t.workspace_id AND s.workflow_id=t.workflow_id AND s.id=t.status WHERE t.id='migration-task' AND i.id='migration-integration'`).Scan(&preserved); err != nil || !preserved {
		t.Fatal("task/integration not preserved and rebound", preserved, err)
	}
	if _, err = tx.Exec(ctx, "SET CONSTRAINTS ALL IMMEDIATE"); err != nil {
		t.Fatal("round trip constraints", err)
	}
}
