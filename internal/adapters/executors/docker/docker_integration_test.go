//go:build docker

package docker

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/adapters/runnerclient/agentctlhttp"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	"github.com/moby/moby/client"
)

func TestDockerAgentctlLaunchSmoke(t *testing.T) {
	image := os.Getenv("AGENTCTL_OPENCODE_IMAGE")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	repoRoot, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	tempRoot := filepath.Join(repoRoot, "tmp")
	if err = os.MkdirAll(tempRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	dataRoot, err := os.MkdirTemp(tempRoot, "docker-smoke-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if removeErr := os.RemoveAll(dataRoot); removeErr != nil {
			t.Errorf("remove smoke-test data directory: %v", removeErr)
		}
	})
	executor, err := New(ctx, Config{DataRoot: dataRoot, Images: map[coreruntime.Runtime]string{coreruntime.OpenCode: image}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = executor.Close() }()

	run := orchestrationmodel.RunConfig{RuntimeConfig: orchestrationmodel.RuntimeConfig{OrganizationID: "docker-test-org", WorkspaceID: "docker-test-workspace", TaskID: "docker-test-task", RunID: "docker-test-run", Runtime: "opencode", Model: orchestrationmodel.ModelSelection{Provider: "provider", ID: "model"}, Agent: orchestrationmodel.AgentDefinition{Name: "Test", Description: "Test", Instructions: "test"}}}
	process, launchErr := executor.StartAgentctl(ctx, run)
	if process.ID != "" {
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()
			engine, clientErr := client.New(client.FromEnv)
			if clientErr != nil {
				t.Errorf("create Docker client for cleanup: %v", clientErr)
				return
			}
			defer func() { _ = engine.Close() }()
			if _, removeErr := engine.ContainerRemove(cleanupCtx, process.ID, client.ContainerRemoveOptions{Force: true}); removeErr != nil {
				t.Errorf("remove smoke-test container: %v", removeErr)
			}
		})
	}
	if launchErr != nil {
		t.Fatal(launchErr)
	}

	healthCtx, healthCancel := context.WithTimeout(ctx, 30*time.Second)
	defer healthCancel()
	if err = agentctlhttp.NewHTTPAgentctlClient(&http.Client{Timeout: 2 * time.Second}).WaitHealthy(healthCtx, process.Endpoint); err != nil {
		t.Fatalf("agentctl health check failed: %v", err)
	}
}
