package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testWorkspaceHostMountsRepair(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
INSERT INTO organizations(id,name) VALUES('mount-repair-org','Repair');
INSERT INTO workspaces(id,organization_id,name) VALUES('mount-repair-workspace','mount-repair-org','Repair');
INSERT INTO workspace_host_mounts(workspace_id,mounts)
VALUES('mount-repair-workspace','[{"source":"/tmp/fixture","target":"/fixture","readOnly":true}]');
DELETE FROM goose_db_version WHERE version_id>=46;
`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := pool.Exec(ctx, `DELETE FROM organizations WHERE id='mount-repair-org'`); err != nil {
			t.Error(err)
		}
	}()
	if err := runMigrations(ctx, pool); err != nil {
		t.Fatal("repair current host mounts schema", err)
	}
	var preserved bool
	if err := pool.QueryRow(ctx, `SELECT mounts='[{"source":"/tmp/fixture","target":"/fixture","readOnly":true}]'::jsonb FROM workspace_host_mounts WHERE workspace_id='mount-repair-workspace'`).Scan(&preserved); err != nil || !preserved {
		t.Fatalf("existing mounts changed: preserved=%v err=%v", preserved, err)
	}
	// Model the version collision in a disposable schema, never application data.
	if _, err := pool.Exec(ctx, `DROP TABLE workspace_host_mounts; DELETE FROM goose_db_version WHERE version_id>=46;`); err != nil {
		t.Fatal(err)
	}
	if err := runMigrations(ctx, pool); err != nil {
		t.Fatal("repair missing host mounts table", err)
	}
	repo := NewWorkspaceHostMountsRepository(&PostgresDatabase{pool: pool})
	mounts, err := repo.List(ctx, "mount-repair-workspace")
	if err != nil || len(mounts) != 0 {
		t.Fatalf("resolve unconfigured host mounts after repair: %v %v", mounts, err)
	}
}
