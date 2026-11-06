//go:build docker

package docker

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	"github.com/josemiguelmelo/orkest/internal/adapters/runnerclient/agentctlhttp"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	mobyclient "github.com/moby/moby/client"
)

// TestDockerSessionRestore checks real runtime storage across daemon restart,
// container replacement, and explicit fresh-session creation without model calls.
func TestDockerSessionRestore(t *testing.T) {
	provider, model := os.Getenv("ORKEST_DOCKER_TEST_MODEL_PROVIDER"), os.Getenv("ORKEST_DOCKER_TEST_MODEL_ID")
	if provider == "" || model == "" {
		t.Fatal("set ORKEST_DOCKER_TEST_MODEL_PROVIDER and ORKEST_DOCKER_TEST_MODEL_ID to an available model; this storage test makes no model calls")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	repoRoot, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(repoRoot, "tmp")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	dataRoot, err := os.MkdirTemp(root, "docker-restore-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dataRoot); err != nil {
			t.Error(err)
		}
	})
	image := os.Getenv("AGENTCTL_OPENCODE_IMAGE")
	executor, err := New(ctx, Config{DataRoot: dataRoot, Images: map[coreruntime.Runtime]string{coreruntime.OpenCode: image}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = executor.Close() }()
	httpClient := agentctlhttp.NewHTTPAgentctlClient(&http.Client{Timeout: 35 * time.Second})
	run := orchestrationmodel.RunConfig{RuntimeConfig: orchestrationmodel.RuntimeConfig{OrganizationID: "restore-org", WorkspaceID: "restore-workspace", TaskID: "restore-task", TaskSessionID: "session-one", RunID: "run-one", Runtime: "opencode", Model: orchestrationmodel.ModelSelection{Provider: provider, ID: model}, Agent: orchestrationmodel.AgentDefinition{Name: "Test", Instructions: "test"}}}
	start := func() orchestrationmodel.Process {
		t.Helper()
		process, err := executor.StartAgentctl(ctx, run)
		if err != nil {
			t.Fatal(err)
		}
		id := process.ID
		t.Cleanup(func() {
			cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
			defer stop()
			if _, err := executor.client.ContainerRemove(cleanupCtx, id, mobyclient.ContainerRemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
				t.Error(err)
			}
		})
		if err := httpClient.WaitHealthy(ctx, process.Endpoint); err != nil {
			t.Fatal(err)
		}
		if err := httpClient.Configure(ctx, process.Endpoint, run); err != nil {
			t.Fatal(err)
		}
		return process
	}
	first := start()
	if err := httpClient.StartAgent(ctx, first.Endpoint); err != nil {
		t.Fatal(err)
	}
	before, err := httpClient.Status(ctx, first.Endpoint)
	if err != nil || before.SessionID == "" {
		t.Fatal("missing runtime identity", err)
	}
	workDir, _, err := executor.taskDirectories(run)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(workDir, "untracked.txt")
	if err := os.WriteFile(file, []byte("retained"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.client.ContainerStop(ctx, first.ID, mobyclient.ContainerStopOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.client.ContainerStart(ctx, first.ID, mobyclient.ContainerStartOptions{}); err != nil {
		t.Fatal(err)
	}
	inspection, err := executor.client.ContainerInspect(ctx, first.ID, mobyclient.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	first.Endpoint, err = publishedEndpoint(inspection.Container, executor.config.AgentctlPort)
	if err != nil {
		t.Fatal(err)
	}
	if err := httpClient.WaitHealthy(ctx, first.Endpoint); err != nil {
		t.Fatal(err)
	}
	after, err := httpClient.Status(ctx, first.Endpoint)
	if err != nil || !after.ProcessAlive || after.SessionID != before.SessionID {
		t.Fatal("daemon did not restore session", after, err)
	}
	if err := executor.TerminateAgentctl(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	run.RunID, run.RestoreSession = "run-two", true
	second := start()
	if err := httpClient.RestoreAgent(ctx, second.Endpoint); err != nil {
		t.Fatal(err)
	}
	after, err = httpClient.Status(ctx, second.Endpoint)
	if err != nil || after.SessionID != before.SessionID {
		t.Fatal("container replacement changed session", err)
	}
	if contents, err := os.ReadFile(file); err != nil || string(contents) != "retained" {
		t.Fatal("working files lost", err)
	}
	if err := executor.TerminateAgentctl(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	run.RunID, run.TaskSessionID, run.RestoreSession = "run-three", "session-two", false
	third := start()
	if err := httpClient.StartAgent(ctx, third.Endpoint); err != nil {
		t.Fatal(err)
	}
	fresh, err := httpClient.Status(ctx, third.Endpoint)
	if err != nil || fresh.SessionID == before.SessionID {
		t.Fatal("new session reused old context", err)
	}
}
