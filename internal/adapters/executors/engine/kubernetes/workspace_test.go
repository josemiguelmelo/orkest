package kubernetes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
)

func TestWorkspaceResolverPreparesLegacyPVCPaths(t *testing.T) {
	root := t.TempDir()
	resolver, err := NewWorkspaceResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	scope := engine.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}
	workspace, err := resolver.ResolveWorkspace(context.Background(), engine.WorkspaceRequest{
		Scope: scope, Intent: engine.WorkspaceEnsureForAgent,
		Required: engine.WorkspaceGuarantees{DurableWork: true, DurableState: true, PlatformSetup: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	work, state := taskstorage.Directories(root, scope.OrganizationID, scope.WorkspaceID, scope.TaskID)
	if !workspace.Exists || workspace.WorkPath != work || workspace.PreparationDirectory != work || workspace.StatePath != state {
		t.Fatalf("resolved workspace = %#v", workspace)
	}
	if workspace.CompatibilityID != filepath.Base(filepath.Dir(work)) {
		t.Fatalf("compatibility ID = %q; want %q", workspace.CompatibilityID, filepath.Base(filepath.Dir(work)))
	}
	for _, path := range []string{work, state} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Mode().Perm() != 0o700 {
			t.Errorf("workspace permissions for %s = %04o", path, info.Mode().Perm())
		}
	}
}

func TestWorkspaceResolverInspectDoesNotCreateWorkspace(t *testing.T) {
	root := t.TempDir()
	resolver, err := NewWorkspaceResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := resolver.ResolveWorkspace(context.Background(), engine.WorkspaceRequest{
		Scope:  engine.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"},
		Intent: engine.WorkspaceInspect,
	})
	if err != nil {
		t.Fatal(err)
	}
	if workspace.Exists {
		t.Fatalf("inspect reported a missing workspace as present: %#v", workspace)
	}
	if _, err = os.Stat(filepath.Join(root, "tasks")); !os.IsNotExist(err) {
		t.Fatalf("inspect mutated task storage, stat error = %v", err)
	}
}
