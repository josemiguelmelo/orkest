package integration

import (
	"context"
	"os"
	"testing"
	"time"

	sharedacp "github.com/josemiguelmelo/orkest/internal/adapters/runtime/acp"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/opencode"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/runtimehost"
	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
)

// TestOpenCodeHandshake needs the pinned executable but makes no model request.
func TestOpenCodeHandshake(t *testing.T) {
	binary := os.Getenv("ORKEST_OPENCODE_BINARY")
	if binary == "" {
		t.Fatal("set ORKEST_OPENCODE_BINARY to the pinned OpenCode executable")
	}
	workDir, stateDir := t.TempDir(), t.TempDir()
	host := runtimehost.New(opencode.Profile{Binary: binary}, sharedacp.Factory{}, runtimehost.Environment{WorkDir: workDir, StateDir: stateDir, GrantedEnv: []string{"PATH=" + os.Getenv("PATH")}})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	config := configmodel.RunConfig{RuntimeConfig: configmodel.RuntimeConfig{OrganizationID: "integration-org", WorkspaceID: "integration-workspace", TaskID: "integration-task", TaskSessionID: "integration-session", RunID: "integration-run", Runtime: "opencode", Agent: configmodel.AgentDefinition{Name: "Integration", Description: "Integration test", Instructions: "You are a test agent."}, Model: configmodel.ModelSelection{Provider: "opencode", ID: "big-pickle"}}}
	process, err := host.Launch(ctx, config.ForRuntime())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()
	if process.SessionID() == "" {
		t.Fatal("missing session identity")
	}
	if err := process.Close(); err != nil {
		t.Fatal(err)
	}
	config.RunID = "integration-revision"
	restored, err := host.Restore(ctx, config.ForRuntime())
	if err != nil {
		t.Fatalf("restore real OpenCode session: %v", err)
	}
	defer func() { _ = restored.Close() }()
	if restored.SessionID() != process.SessionID() {
		t.Fatal("restoration created a replacement session")
	}
}
