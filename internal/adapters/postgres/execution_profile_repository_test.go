package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	executionprofilemodel "github.com/josemiguelmelo/orkest/internal/server/executionprofile/model"
)

func testExecutionProfiles(t *testing.T, db *PostgresDatabase, ws string) {
	t.Helper()
	ctx := context.Background()
	r := NewExecutionProfileRepository(db)
	p := executionprofilemodel.Profile{ID: ws + "-profile", WorkspaceID: ws, Input: executionprofilemodel.Input{Name: "Java", Runtime: "opencode", ToolchainImage: "example.com/java:21"}}
	b := executionprofilemodel.Build{ID: ws + "-build-1", Runtime: "opencode", RuntimeVersion: "opencode v2.1.4", ToolchainImage: p.ToolchainImage, BaseImage: "example.com/agentctl:1", Destination: "example.com/output:build-1"}
	saved, err := r.Save(ctx, p, b, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Get(ctx, "another-workspace", p.ID); !errors.Is(err, modelerrors.ErrNotFound) {
		t.Fatal("cross-workspace profile read", err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	claims := []executionprofilemodel.Build{}
	for range 8 {
		wg.Go(func() {
			build, ok, err := r.Claim(ctx, "worker")
			if err != nil {
				t.Error(err)
			}
			if ok {
				mu.Lock()
				claims = append(claims, build)
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if len(claims) != 1 {
		t.Fatalf("expected single claim, got %d", len(claims))
	}
	p.Generation = saved.Generation
	p.ToolchainImage = "example.com/java:22"
	b.ID = ws + "-build-2"
	updated, err := r.Save(ctx, p, b, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Save(ctx, p, b, false); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatal("stale generation accepted", err)
	}
	if err = r.Finish(ctx, claims[0], "ready", "example.com/output@sha256:old", ""); err != nil {
		t.Fatal(err)
	}
	current, err := r.Get(ctx, ws, p.ID)
	if err != nil || current.ExecutionImage != "" || current.Generation != updated.Generation {
		t.Fatal("stale artifact promoted", current, err)
	}
	next, ok, err := r.Claim(ctx, "new-worker")
	if err != nil || !ok {
		t.Fatal("missing new attempt", err)
	}
	if err = r.Finish(ctx, next, "ready", "example.com/output@sha256:new", ""); err != nil {
		t.Fatal(err)
	}
	current, err = r.Get(ctx, ws, p.ID)
	if err != nil || current.ExecutionImage != "example.com/output@sha256:new" {
		t.Fatal("artifact not promoted", err)
	}
	p.Generation = updated.Generation
	b.ID = ws + "-build-3"
	if _, err = r.Save(ctx, p, b, false); err != nil {
		t.Fatal(err)
	}
	failed, _, err := r.Claim(ctx, "failed-worker")
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Finish(ctx, failed, "failed", "", "build failed"); err != nil {
		t.Fatal(err)
	}
	current, err = r.Get(ctx, ws, p.ID)
	if err != nil || current.ExecutionImage != "example.com/output@sha256:new" {
		t.Fatal("failed update removed previous image", err)
	}
}
