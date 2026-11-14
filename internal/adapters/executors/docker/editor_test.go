package docker

import (
	"context"
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
	backend := &EditorBackend{executor: &Executor{config: Config{DataRoot: root}}}
	_, err = backend.StartEditor(context.Background(), taskeditor.Spec{Instance: taskeditor.Instance{Scope: taskeditor.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}, Mode: taskeditor.ReadOnly}, Discovery: taskeditor.Discovery{Roots: []taskeditor.Root{{Source: source, Path: "/workspace/repositories/github/repo"}}}})
	if err == nil {
		t.Fatal("editor accepted a checkout from another workspace")
	}
}
