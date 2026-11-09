package postgres

import (
	"context"
	"reflect"
	"testing"

	mountmodel "github.com/josemiguelmelo/orkest/internal/server/workspacemount/model"
)

func testWorkspaceHostMounts(t *testing.T, database *PostgresDatabase, workspaceID string) {
	t.Helper()
	ctx := context.Background()
	repository := NewWorkspaceHostMountsRepository(database)
	mounts, err := repository.List(ctx, workspaceID)
	if err != nil || mounts == nil || len(mounts) != 0 {
		t.Fatalf("initial mounts=%+v err=%v", mounts, err)
	}
	expected := []mountmodel.Mount{
		{Source: "/srv/input", Target: "/mnt/host/input", ReadOnly: true},
		{Source: "/srv/output", Target: "/mnt/host/output", ReadOnly: false},
	}
	if err := repository.Put(ctx, workspaceID, expected); err != nil {
		t.Fatal(err)
	}
	mounts, err = repository.List(ctx, workspaceID)
	if err != nil || !reflect.DeepEqual(mounts, expected) {
		t.Fatalf("mounts=%+v err=%v", mounts, err)
	}
	other, err := repository.List(ctx, "another-workspace")
	if err != nil || len(other) != 0 {
		t.Fatal("mounts crossed workspace boundary", err)
	}
	if err := repository.Put(ctx, "missing-workspace", expected); err == nil {
		t.Fatal("mounts persisted without workspace")
	}
	if err := repository.Put(ctx, workspaceID, expected[:1]); err != nil {
		t.Fatal(err)
	}
	mounts, err = repository.List(ctx, workspaceID)
	if err != nil || !reflect.DeepEqual(mounts, expected[:1]) {
		t.Fatal("replacement did not persist", err)
	}
	if err := repository.Put(ctx, workspaceID, nil); err != nil {
		t.Fatal(err)
	}
	mounts, err = repository.List(ctx, workspaceID)
	if err != nil || mounts == nil || len(mounts) != 0 {
		t.Fatal("clear did not persist as an array", err)
	}
}
