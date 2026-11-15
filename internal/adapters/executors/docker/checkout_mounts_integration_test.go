//go:build docker

package docker

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
	client "github.com/moby/moby/client"
)

// legacyMountEngine reproduces a persisted worker from before parent protection.
// It is used only to prove admission fails without stopping that worker.
type legacyMountEngine struct{ EngineClient }

func (e legacyMountEngine) ContainerCreate(ctx context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	delete(options.Config.Labels, checkoutMountLayoutLabel)
	mounts := options.HostConfig.Mounts[:0]
	for _, m := range options.HostConfig.Mounts {
		if !strings.HasPrefix(m.Target, workMountPath+"/repositories") {
			mounts = append(mounts, m)
		}
	}
	options.HostConfig.Mounts = mounts
	return e.EngineClient.ContainerCreate(ctx, options)
}

func TestDockerProtectedCheckoutMounts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	base, err := filepath.Abs("../../../../tmp")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(base, "protected-checkouts-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	run := runConfig()
	run.RunID = "protected-" + filepath.Base(root)
	run.Repositories = []orchestrationmodel.WorkspaceRepository{{ID: "repo", ProviderName: "test", Name: "repo", URL: "https://example.invalid/repo.git", IntegrationID: "integration", ProviderRepositoryID: "repo", DefaultBranch: "main"}}
	work, _ := taskstorage.Directories(root, run.OrganizationID, run.WorkspaceID, run.TaskID)
	repo := filepath.Join(work, "repositories", "test", "repo")
	initInspectorCheckout(t, repo)
	executor, err := New(ctx, Config{DataRoot: root, CodingCredentialResolver: &credentialResolverStub{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = executor.Close() }()
	executor.repositoryProviders["test"] = &checkoutProviderStub{}
	backend, err := executor.NewEditorBackend(DefaultEditorImage, DefaultReadonlyEditorImage)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := (TaskCheckoutInspector{DataRoot: root}).Inspect(ctx, run.OrganizationID, run.WorkspaceID, run.TaskID, []taskeditor.Checkout{{RegistrationID: "repo", ProviderName: "test", Name: "repo"}})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := json.Marshal(struct {
		Folders []taskeditor.Root `json:"folders"`
	}{inspection.Roots})
	if err != nil {
		t.Fatal(err)
	}
	spec := taskeditor.Spec{Instance: taskeditor.Instance{Scope: taskeditor.Scope{OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, TaskID: run.TaskID}, UserID: "user", Generation: strings.Repeat("b", 64), Mode: taskeditor.ReadOnly}, Discovery: taskeditor.Discovery{Roots: inspection.Roots, Manifest: manifest}}
	engine := executor.client
	executor.client = legacyMountEngine{EngineClient: engine}
	legacy, err := executor.StartAgentctl(ctx, run)
	executor.client = engine
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		_ = executor.TerminateAgentctl(cleanup, legacy.ID)
	}()
	if _, err := backend.StartEditor(ctx, spec); !errors.Is(err, taskeditor.ErrUnavailable) {
		t.Fatalf("legacy worker admitted editor: %v", err)
	}
	if running, err := executor.AgentctlRunning(ctx, legacy.ID); err != nil || !running {
		t.Fatalf("legacy worker disturbed: %v", err)
	}
	if err := executor.TerminateAgentctl(ctx, legacy.ID); err != nil {
		t.Fatal(err)
	}
	run.RunID += "-replacement"
	worker, err := executor.StartAgentctl(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		_ = executor.TerminateAgentctl(cleanup, worker.ID)
	}()
	for _, path := range []string{"/workspace/repositories", "/workspace/repositories/test", "/workspace/repositories/test/repo"} {
		output, err := exec.CommandContext(ctx, "docker", "exec", worker.ID, "mv", path, path+".moved").CombinedOutput()
		var exit *exec.ExitError
		denial := strings.ToLower(string(output))
		if !errors.As(err, &exit) || exit.ExitCode() != 1 || (!strings.Contains(denial, "read-only file system") && !strings.Contains(denial, "device or resource busy")) {
			t.Fatalf("mount protection did not deny rename %s: %v %s", path, err, output)
		}
	}
	if output, err := exec.CommandContext(ctx, "docker", "exec", worker.ID, "bash", "-c", "printf retained > /workspace/repositories/test/repo/untracked.txt; git -C /workspace/repositories/test/repo checkout -b retained-branch").CombinedOutput(); err != nil {
		t.Fatalf("checkout not writable: %v %s", err, output)
	}
	editor, err := backend.StartEditor(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		_ = backend.StopEditor(cleanup, editor)
	}()
	if output, err := exec.CommandContext(ctx, "docker", "exec", editor.ID, "cat", "/task/repositories/test/repo/untracked.txt").CombinedOutput(); err != nil || string(output) != "retained" {
		t.Fatalf("readonly editor lost worker changes: %v %s", err, output)
	}
	if branch := runGit(t, repo, "branch", "--show-current"); strings.TrimSpace(branch) != "retained-branch" {
		t.Fatalf("branch lost: %s", branch)
	}
}
