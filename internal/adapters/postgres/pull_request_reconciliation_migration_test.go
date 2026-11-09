package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
)

func testPullRequestReconciliationRepair(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	// This pool uses the migration suite's disposable schema, never product data.
	if _, err := pool.Exec(ctx, `
INSERT INTO organizations(id,name) VALUES('repair-org','Repair');
INSERT INTO workspaces(id,organization_id,name) VALUES('repair-workspace','repair-org','Repair');
INSERT INTO task_trigger_integrations(id,workspace_id,name,provider,config,enabled,configured_by,encrypted_secret)
VALUES('repair-integration','repair-workspace','Repair','github','{}',true,'fixture','encrypted-fixture');
INSERT INTO task_pull_request_reconciliation(integration_id,configuration_version,diagnostic)
VALUES('repair-integration',17,'keep existing state');
DELETE FROM goose_db_version WHERE version_id>=44;
`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := pool.Exec(ctx, `DELETE FROM organizations WHERE id='repair-org'`); err != nil {
			t.Error(err)
		}
	}()
	if err := runMigrations(ctx, pool); err != nil {
		t.Fatal("upgrade with existing reconciliation table", err)
	}
	var version int64
	var diagnostic string
	if err := pool.QueryRow(ctx, `SELECT configuration_version,diagnostic FROM task_pull_request_reconciliation WHERE integration_id='repair-integration'`).Scan(&version, &diagnostic); err != nil || version != 17 || diagnostic != "keep existing state" {
		t.Fatalf("existing reconciliation state changed: version=%d diagnostic=%q err=%v", version, diagnostic, err)
	}
	// Reproduce the historical version-43 ledger with its reconciliation table absent.
	if _, err := pool.Exec(ctx, `DROP TABLE task_pull_request_reconciliation; DELETE FROM goose_db_version WHERE version_id>=44;`); err != nil {
		t.Fatal(err)
	}
	if err := runMigrations(ctx, pool); err != nil {
		t.Fatal("repair historical migration 43", err)
	}
	// This is the same query used when saving a trigger integration.
	if err := sqlc.New(pool).ResetPullRequestDiscovery(ctx, "repair-integration"); err != nil {
		t.Fatal("save discovery state after repair", err)
	}
	if err := pool.QueryRow(ctx, `SELECT configuration_version,diagnostic FROM task_pull_request_reconciliation WHERE integration_id='repair-integration'`).Scan(&version, &diagnostic); err != nil || version != 1 || diagnostic != "" {
		t.Fatalf("repaired discovery state: version=%d diagnostic=%q err=%v", version, diagnostic, err)
	}
}
