//go:build docker

package docker

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
	client "github.com/moby/moby/client"
)

func TestDockerEditorLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	root, err := filepath.Abs("../../../../tmp")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	dataRoot, err := os.MkdirTemp(root, "editor-backend-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dataRoot) }()
	work, _ := taskstorage.Directories(dataRoot, "org", "workspace", "task")
	repo := filepath.Join(work, "repositories", "github", "repo")
	initInspectorCheckout(t, repo)
	// Fixtures are readable by the non-root container UID on Docker Desktop/Colima.
	// Production discovery never changes checkout ownership or permissions.
	if err = filepath.WalkDir(dataRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.Chmod(path, 0o755)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	executor, err := New(ctx, Config{DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = executor.Close() }()
	image := os.Getenv("ORKEST_EDITOR_IMAGE")
	if image == "" {
		image = "orkest/task-editor:4.145.2"
	}
	readonly := os.Getenv("ORKEST_EDITOR_READONLY_IMAGE")
	if readonly == "" {
		readonly = image + "-ro"
	}
	backend, err := executor.NewEditorBackend(image, readonly)
	if err != nil {
		t.Fatal(err)
	}
	inspected, err := (TaskCheckoutInspector{DataRoot: dataRoot}).Inspect(ctx, "org", "workspace", "task", []taskeditor.Checkout{{RegistrationID: "repo", ProviderName: "github", Name: "repo"}})
	if err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.Marshal(struct {
		Folders []taskeditor.Root `json:"folders"`
	}{inspected.Roots})
	scope := taskeditor.Scope{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}
	for _, mode := range []taskeditor.Mode{taskeditor.ReadWrite, taskeditor.ReadOnly} {
		instance, startErr := backend.StartEditor(ctx, taskeditor.Spec{Instance: taskeditor.Instance{Scope: scope, UserID: "user", Generation: strings.Repeat("a", 64), Fingerprint: "selection", Mode: mode}, Discovery: taskeditor.Discovery{Roots: inspected.Roots, Manifest: manifest}})
		// Always clean up by discovered concrete identity, including uncertain starts.
		defer func() {
			cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
			defer stop()
			i, found, err := backend.InspectEditor(cleanupCtx, scope)
			if err == nil && found {
				_ = backend.StopEditor(cleanupCtx, i)
			}
		}()
		if startErr != nil {
			t.Fatal(startErr)
		}
		command := exec.CommandContext(ctx, "docker", "exec", instance.ID, "bash", "-c", "echo saved > /task/repositories/github/repo/saved.txt")
		output, writeErr := command.CombinedOutput()
		if mode == taskeditor.ReadWrite && writeErr != nil {
			t.Fatalf("write: %v %s", writeErr, output)
		}
		if mode == taskeditor.ReadOnly && writeErr == nil {
			t.Fatal("readonly mount accepted write")
		}
		discovered, err := backend.ListEditors(ctx)
		if err != nil || len(discovered) != 1 || discovered[0].ID != instance.ID {
			t.Fatalf("discovery: %+v %v", discovered, err)
		}
		if mode == taskeditor.ReadOnly {
			relay, err := executor.client.ContainerInspect(ctx, backend.relayName(instance), client.ContainerInspectOptions{})
			if err != nil {
				t.Fatal(err)
			}
			timeout := 1
			if _, err = executor.client.ContainerStop(ctx, relay.Container.ID, client.ContainerStopOptions{Timeout: &timeout}); err != nil {
				t.Fatal(err)
			}
			broken, found, err := backend.InspectEditor(ctx, scope)
			if err != nil || !found || broken.Endpoint != "" {
				t.Fatalf("failed relay was not inspectable for cleanup: %v", err)
			}
		}
		if err = backend.StopEditor(ctx, instance); err != nil {
			t.Fatal(err)
		}
		if _, found, err := backend.InspectEditor(ctx, scope); err != nil || found {
			t.Fatalf("stop not confirmed: %v", err)
		}
		if saved, err := os.ReadFile(filepath.Join(repo, "saved.txt")); err != nil || string(saved) != "saved\n" {
			t.Fatalf("saved file lost: %v", err)
		}
	}
}
