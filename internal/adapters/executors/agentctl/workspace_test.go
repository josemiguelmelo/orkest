package agentctl

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

func TestPrepareWorkspacePreservesHistoricalPathsAndPermissions(t *testing.T) {
	root := t.TempDir()
	workspace, err := PrepareWorkspace(context.Background(), WorkspaceConfig{DataRoot: root}, model.RunConfig{
		RuntimeConfig: model.RuntimeConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"},
	})
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(root, "tasks", "961e0f4564a3db8eeae39c31bea4d9026bb33bc064377d6252b33e63ee4224e4")
	if workspace.WorkPath != filepath.Join(base, "work") || workspace.StatePath != filepath.Join(base, "agentctl-state") {
		t.Fatalf("workspace paths = %q %q", workspace.WorkPath, workspace.StatePath)
	}
	if workspace.PreparationDirectory != workspace.WorkPath || !workspace.Exists {
		t.Fatalf("workspace preparation result = %+v", workspace)
	}
	for _, path := range []string{workspace.WorkPath, workspace.StatePath} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Mode().Perm() != 0o700 {
			t.Errorf("permissions for %s = %o, want 700", path, info.Mode().Perm())
		}
	}
}

func TestBuildArgumentsPreservesNativeAndContainerRunIdentityRules(t *testing.T) {
	native := BuildArguments(ArgumentConfig{
		Runtime: "opencode", ListenAddress: "127.0.0.1:1234", WorkPath: "/work",
		StatePath: "/state", RunID: "run-1", Native: true, EnvironmentNames: []string{"TOKEN", "TOKEN"},
	})
	wantNative := []string{"-listen", "127.0.0.1:1234", "-runtime", "opencode", "-work-dir", "/work", "-state-dir", "/state", "-run-id", "run-1", "-env", "TOKEN", "-env", "PATH"}
	if !equalStrings(native, wantNative) {
		t.Fatalf("native args = %q, want %q", native, wantNative)
	}
	container := BuildArguments(ArgumentConfig{
		Runtime: "opencode", ListenAddress: "0.0.0.0:8081", WorkPath: "/workspace",
		StatePath: "/agentctl-state", RunID: "run-2", HasSession: true, EnvironmentNames: []string{"TOKEN"},
	})
	wantContainer := []string{"-listen", "0.0.0.0:8081", "-runtime", "opencode", "-work-dir", "/workspace", "-state-dir", "/agentctl-state", "-run-id", "run-2", "-env", "TOKEN"}
	if !equalStrings(container, wantContainer) {
		t.Fatalf("container args = %q, want %q", container, wantContainer)
	}
	withoutSession := BuildArguments(ArgumentConfig{Runtime: "opencode", ListenAddress: "0.0.0.0:8081", RunID: "run-3"})
	if contains(withoutSession, "-run-id") {
		t.Fatalf("container args without session included run ID: %q", withoutSession)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
