//go:build docker

package integration

import (
	"context"
	"os/exec"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	sharedagentctl "github.com/josemiguelmelo/orkest/internal/adapters/executors/agentctl"
	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	dockerengine "github.com/josemiguelmelo/orkest/internal/adapters/executors/engine/docker"
	"github.com/josemiguelmelo/orkest/internal/adapters/runnerclient/agentctlhttp"
	runtimecatalog "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	mountmodel "github.com/josemiguelmelo/orkest/internal/server/workspacemount/model"
)

func TestDockerEngineSharedAgentctlLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client, err := dockerengine.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	dataParent := os.Getenv("ORKEST_DOCKER_TEST_DATA_ROOT")
	if dataParent == "" {
		dataParent, err = filepath.Abs("../../tmp")
	}
	if err != nil {
		_ = client.Close()
		t.Fatal(err)
	}
	if err = os.MkdirAll(dataParent, 0o700); err != nil {
		_ = client.Close()
		t.Fatal(err)
	}
	dataRoot, err := os.MkdirTemp(dataParent, "executor-engine-agent-")
	if err != nil {
		_ = client.Close()
		t.Fatal(err)
	}
	defer func() {
		if removeErr := os.RemoveAll(dataRoot); removeErr != nil {
			t.Errorf("remove agent integration data: %v", removeErr)
		}
	}()
	infrastructure, err := dockerengine.New(dockerengine.Config{DataRoot: dataRoot, Client: client})
	if err != nil {
		_ = client.Close()
		t.Fatal(err)
	}
	defer func() { _ = infrastructure.Close() }()
	image := os.Getenv("AGENTCTL_OPENCODE_IMAGE")
	if image == "" {
		image = (runtimecatalog.OpenCodeProvider{}).DefaultImage()
	}
	executor, err := sharedagentctl.NewSharedAgentCtlExecutor(sharedagentctl.ExecutorConfig{Engine: infrastructure, Mode: engine.ExecutionModeContainer, Images: map[runtimecatalog.Runtime]string{runtimecatalog.OpenCode: image}})
	if err != nil {
		t.Fatal(err)
	}
	run := model.RunConfig{RuntimeConfig: model.RuntimeConfig{
		OrganizationID: "executor-integration", WorkspaceID: "docker-agent", TaskID: "task", RunID: "docker-engine-agent-run", Runtime: runtimecatalog.OpenCode,
		Model: model.ModelSelection{Provider: "provider", ID: "model"}, Agent: model.AgentDefinition{Name: "Engine integration", Description: "Engine integration", Instructions: "test"},
	}}
	process, err := executor.StartAgentctl(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if stopErr := executor.TerminateAgentctl(cleanupCtx, process.ID); stopErr != nil {
			t.Error(stopErr)
		}
	}()
	healthCtx, healthCancel := context.WithTimeout(ctx, 60*time.Second)
	defer healthCancel()
	if err = agentctlhttp.NewHTTPAgentctlClient(&http.Client{Timeout: 2 * time.Second}).WaitHealthy(healthCtx, process.Endpoint); err != nil {
		t.Fatalf("agentctl health check failed: %v", err)
	}
}

func TestDockerEngineSharedAgentctlHostMounts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	root := integrationDataRoot(t, "executor-engine-host-mount-")
	input, output := filepath.Join(root, "input"), filepath.Join(root, "output")
	for _, directory := range []string{input, output} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(input, "data.txt"), []byte("host-input"), 0o600); err != nil {
		t.Fatal(err)
	}
	client, err := dockerengine.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	infrastructure, err := dockerengine.New(dockerengine.Config{DataRoot: root, Client: client})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = infrastructure.Close() }()
	image := os.Getenv("AGENTCTL_OPENCODE_IMAGE")
	if image == "" {
		image = (runtimecatalog.OpenCodeProvider{}).DefaultImage()
	}
	executor, err := sharedagentctl.NewSharedAgentCtlExecutor(sharedagentctl.ExecutorConfig{Engine: infrastructure, Mode: engine.ExecutionModeContainer, Images: map[runtimecatalog.Runtime]string{runtimecatalog.OpenCode: image}})
	if err != nil {
		t.Fatal(err)
	}
	run := model.RunConfig{RuntimeConfig: model.RuntimeConfig{
		OrganizationID: "executor-integration", WorkspaceID: "docker-host-mount", TaskID: "task", RunID: "docker-engine-host-mount-run", Runtime: runtimecatalog.OpenCode,
		Model: model.ModelSelection{Provider: "provider", ID: "model"}, Agent: model.AgentDefinition{Name: "Host mount integration", Instructions: "test"},
	}}
	run.HostMounts = []mountmodel.Mount{
		{Source: input, Target: "/mnt/host/input", ReadOnly: true},
		{Source: output, Target: "/mnt/host/output"},
	}
	process, err := executor.StartAgentctl(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if stopErr := executor.TerminateAgentctl(cleanupCtx, process.ID); stopErr != nil {
			t.Error(stopErr)
		}
	}()
	script := `set -eu
test "$(cat /mnt/host/input/data.txt)" = host-input
if echo changed > /mnt/host/input/data.txt; then exit 1; fi
echo host-output > /mnt/host/output/result.txt`
	if result, err := exec.CommandContext(ctx, "docker", "exec", process.ID, "/bin/sh", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("shared engine host mount behavior: %v: %s", err, result)
	}
	if data, err := os.ReadFile(filepath.Join(output, "result.txt")); err != nil || string(data) != "host-output\n" {
		t.Fatalf("writable host mount did not reach host: %q, %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(input, "data.txt")); err != nil || string(data) != "host-input" {
		t.Fatalf("read-only host mount changed: %q, %v", data, err)
	}
}

func integrationDataRoot(t *testing.T, pattern string) string {
	t.Helper()
	parent := os.Getenv("ORKEST_DOCKER_TEST_DATA_ROOT")
	if parent == "" {
		var err error
		parent, err = filepath.Abs("../../tmp")
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(parent, pattern)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Errorf("remove integration data root: %v", err)
		}
	})
	return root
}
