//go:build docker

package docker

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	"github.com/josemiguelmelo/orkest/internal/adapters/runnerclient/agentctlhttp"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	mobyclient "github.com/moby/moby/client"
)

// TestDockerSessionRestore checks real runtime storage across daemon restart,
// container replacement, and explicit fresh-session creation without model calls.
func TestDockerSessionRestore(t *testing.T) {
	testDockerSessionRestore(t, false)
}

// Explicit model configuration is required: this test makes two model calls to
// prove a legacy mount layout's session survives replacement and continuation.
func TestDockerLegacyCheckoutContinuation(t *testing.T) {
	testDockerSessionRestore(t, true)
}

func testDockerSessionRestore(t *testing.T, continuation bool) {
	provider, model := os.Getenv("ORKEST_DOCKER_TEST_MODEL_PROVIDER"), os.Getenv("ORKEST_DOCKER_TEST_MODEL_ID")
	if provider == "" || model == "" {
		t.Fatal("set ORKEST_DOCKER_TEST_MODEL_PROVIDER and ORKEST_DOCKER_TEST_MODEL_ID to an explicitly selected model")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
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
	engine := executor.client
	marker := ""
	if continuation {
		marker, err = ids.NewUUID()
		if err != nil {
			t.Fatal(err)
		}
		work, _ := taskstorage.Directories(dataRoot, run.OrganizationID, run.WorkspaceID, run.TaskID)
		initInspectorCheckout(t, filepath.Join(work, "repositories", "test", "repo"))
		executor.config.CodingCredentialResolver = &credentialResolverStub{}
		executor.repositoryProviders["test"] = &checkoutProviderStub{}
		run.Repositories = []orchestrationmodel.WorkspaceRepository{{ID: "repo", ProviderName: "test", Name: "repo", URL: "https://example.invalid/repo.git", IntegrationID: "integration", ProviderRepositoryID: "repo", DefaultBranch: "main"}}
		run.Agent.Instructions = "Follow the user's instructions. Do not use tools for this memory test."
		executor.client = legacyMountEngine{EngineClient: engine}
	}
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
	if continuation {
		dockerMemoryPrompt(t, ctx, httpClient, first.Endpoint, "remember", "Remember this private test marker for our next turn: "+marker+". Reply only acknowledged.")
	}
	workDir, _, err := executor.taskDirectories(run)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(workDir, "untracked.txt")
	if continuation {
		file = filepath.Join(workDir, "repositories", "test", "repo", "untracked.txt")
		runGit(t, filepath.Dir(file), "checkout", "-b", "retained-branch")
	}
	if err := os.WriteFile(file, []byte("retained"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !continuation {
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
	}
	if err := executor.TerminateAgentctl(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	executor.client = engine
	run.RunID, run.RestoreSession = "run-two", true
	second := start()
	if err := httpClient.RestoreAgent(ctx, second.Endpoint); err != nil {
		t.Fatal(err)
	}
	after, err := httpClient.Status(ctx, second.Endpoint)
	if err != nil || after.SessionID != before.SessionID {
		t.Fatal("container replacement changed session", err)
	}
	if continuation {
		answer := dockerMemoryPrompt(t, ctx, httpClient, second.Endpoint, "follow-up", "What exact private test marker did I ask you to remember? Reply with only the marker.")
		if !strings.Contains(answer, marker) {
			t.Fatal("legacy session lost conversation context after mount replacement")
		}
	}
	if contents, err := os.ReadFile(file); err != nil || string(contents) != "retained" {
		t.Fatal("working files lost", err)
	}
	if continuation && strings.TrimSpace(runGit(t, filepath.Dir(file), "branch", "--show-current")) != "retained-branch" {
		t.Fatal("task branch lost")
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

func dockerMemoryPrompt(t *testing.T, ctx context.Context, c *agentctlhttp.HTTPAgentctlClient, endpoint, command, text string) string {
	t.Helper()
	if err := c.Prompt(ctx, endpoint, command, text); err != nil {
		t.Fatal(err)
	}
	completed := errors.New("prompt completed")
	var answer strings.Builder
	err := c.Connect(ctx, endpoint, 0, func() {}, func(event orchestrationmodel.AgentActivityEvent) error {
		if event.CommandID != command {
			return nil
		}
		if event.Update != nil && event.Update.Kind == "text" {
			answer.WriteString(event.Update.Text)
		}
		switch event.Type {
		case orchestrationmodel.EventPromptCompleted:
			return completed
		case orchestrationmodel.EventPromptFailed, orchestrationmodel.EventPermissionRequested, orchestrationmodel.EventQuestionRequested:
			return errors.New("unexpected failure or interaction in memory test")
		}
		return nil
	})
	if !errors.Is(err, completed) {
		t.Fatalf("memory prompt did not complete: %v", err)
	}
	return answer.String()
}
