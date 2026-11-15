package docker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
)

func TestEditorRejectsAnotherTaskMountBeforeEngineCalls(t *testing.T) {
	root := t.TempDir()
	work, _ := taskstorage.Directories(root, "org", "other-workspace", "task")
	repo := filepath.Join(work, "repositories", "github", "repo")
	initInspectorCheckout(t, repo)
	source, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	// A nil Engine proves rejected specs cannot reach Docker resource creation.
	backend := &EditorBackend{config: EditorConfig{DataRoot: root}}
	_, err = backend.StartEditor(context.Background(), taskeditor.Spec{Instance: taskeditor.Instance{Scope: taskeditor.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}, Mode: taskeditor.ReadOnly}, Discovery: taskeditor.Discovery{Roots: []taskeditor.Root{{StorageRef: source, Path: "/workspace/repositories/github/repo"}}}})
	if err == nil {
		t.Fatal("editor accepted a checkout from another workspace")
	}
}

func TestStandaloneEditorRevalidatesTaskStorageReference(t *testing.T) {
	root := t.TempDir()
	work, _ := taskstorage.Directories(root, "org", "workspace", "task")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatal(err)
	}
	// The client has no usable methods: rejection must precede daemon calls.
	backend, err := NewEditorBackendWithClient(EditorConfig{DataRoot: root, ContainerUser: "1000:1000"}, &cleanupEngine{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = backend.StartEditor(context.Background(), taskeditor.Spec{
		Instance:  taskeditor.Instance{Scope: taskeditor.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}, Mode: taskeditor.ReadOnly},
		Discovery: taskeditor.Discovery{Roots: []taskeditor.Root{{StorageRef: root, Path: "/task"}}},
	})
	if err == nil {
		t.Fatal("accepted a storage reference outside the scoped task worktree")
	}
}

func TestStandaloneEditorUsesResolvedConfigurationForDiscovery(t *testing.T) {
	root := t.TempDir()
	first, err := NewEditorBackendWithClient(EditorConfig{DataRoot: root, ContainerUser: "0:0"}, &cleanupEngine{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewEditorBackendWithClient(EditorConfig{DataRoot: root, ContainerUser: "1000:1000", Image: "different-image"}, &cleanupEngine{})
	if err != nil {
		t.Fatal(err)
	}
	if first.config.ContainerUser != "1000:1000" {
		t.Fatal("editor inherited root UID")
	}
	ctx := context.Background()
	a, err := first.Inspect(ctx, "org", "workspace", "task", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := second.Inspect(ctx, "org", "workspace", "task", nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.CompatibilityKey == "" || a.CompatibilityKey == b.CompatibilityKey {
		t.Fatal("changed image did not change private compatibility identity")
	}
}
