//go:build docker && security

package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

// This strict qualification is intentionally separate from the lifecycle smoke
// test: successful editing must never be mistaken for cross-task isolation.
func TestDockerEditorNetworkIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	base, err := filepath.Abs("../../../../tmp")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(base, "editor-security-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	executor, err := New(ctx, Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = executor.Close() }()
	backend, err := executor.NewEditorBackend("orkest/task-editor:4.145.2", "orkest/task-editor:4.145.2-ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		instances, err := backend.ListEditors(cleanupCtx)
		if err != nil {
			t.Error(err)
			return
		}
		for _, instance := range instances {
			if err = backend.StopEditor(cleanupCtx, instance); err != nil {
				t.Error(err)
			}
		}
	}()
	instances := make([]taskeditor.Instance, 0, 2)
	for _, workspace := range []string{"workspace-a", "workspace-b"} {
		work, _ := taskstorage.Directories(root, "org", workspace, "task")
		repo := filepath.Join(work, "repositories", "github", "repo")
		initInspectorCheckout(t, repo)
		if err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
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
		inspection, err := (TaskCheckoutInspector{DataRoot: root}).Inspect(ctx, "org", workspace, "task", []taskeditor.Checkout{{ProviderName: "github", RegistrationID: "repo", Name: "repo"}})
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := json.Marshal(struct {
			Folders []taskeditor.Root `json:"folders"`
		}{inspection.Roots})
		if err != nil {
			t.Fatal(err)
		}
		instance, err := backend.StartEditor(ctx, taskeditor.Spec{Instance: taskeditor.Instance{Scope: taskeditor.Scope{OrganizationID: "org", WorkspaceID: workspace, TaskID: "task"}, UserID: workspace, Generation: strings.Repeat("a", 64), Mode: taskeditor.ReadWrite}, Discovery: taskeditor.Discovery{Roots: inspection.Roots, Manifest: manifest}})
		if err != nil {
			t.Fatal(err)
		}
		instances = append(instances, instance)
	}
	httpClient := &http.Client{Timeout: time.Second}
	for _, instance := range instances {
		response, err := httpClient.Get(instance.Endpoint + "/healthz")
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("unauthenticated relay access: %d", response.StatusCode)
		}
	}
	workerConfig := runConfig()
	workerConfig.RunID = "security-worker-" + filepath.Base(root)
	worker, err := executor.StartAgentctl(ctx, workerConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if err := executor.TerminateAgentctl(cleanupCtx, worker.ID); err != nil {
			t.Error(err)
		}
	}()
	ready := false
	for range 30 {
		response, err := httpClient.Get(worker.Endpoint + "/health")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Fatal("worker fixture did not become healthy")
	}
	workerInfo, err := executor.client.ContainerInspect(ctx, worker.ID, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	targets := []string{}
	for _, endpoint := range workerInfo.Container.NetworkSettings.Networks {
		targets = append(targets, "http://"+fmt.Sprint(endpoint.IPAddress)+":8081/health")
	}
	workerURL, err := url.Parse(worker.Endpoint)
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"host.docker.internal", "host.lima.internal"} {
		targets = append(targets, "http://"+alias+":"+workerURL.Port()+"/health")
	}
	for _, target := range targets {
		probe := exec.CommandContext(ctx, "docker", "exec", instances[0].ID, "curl", "--noproxy", "*", "--max-time", "2", "--fail", "--silent", target)
		if output, err := probe.CombinedOutput(); err == nil {
			t.Fatalf("editor can access worker control plane through %s: %s", target, strings.TrimSpace(string(output)))
		}
	}
	peer, err := executor.client.ContainerInspect(ctx, instances[1].ID, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if peer.Container.NetworkSettings == nil || len(peer.Container.NetworkSettings.Networks) == 0 {
		t.Fatal("missing test peer network identity")
	}
	for _, endpoint := range peer.Container.NetworkSettings.Networks {
		address := "http://" + fmt.Sprint(endpoint.IPAddress) + ":8080/healthz"
		probe := exec.CommandContext(ctx, "docker", "exec", instances[0].ID, "curl", "--noproxy", "*", "--max-time", "3", "--fail", "--silent", address)
		output, err := probe.CombinedOutput()
		if err == nil {
			t.Fatalf("cross-workspace unauthenticated editor API is reachable: %s", strings.TrimSpace(string(output)))
		}
	}
}
