//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtime/opencode"
)

// TestOpenCodeHandshake needs the pinned executable but makes no model request.
func TestOpenCodeHandshake(t *testing.T) {
	binary := os.Getenv("ORKEST_OPENCODE_BINARY")
	if binary == "" {
		t.Fatal("set ORKEST_OPENCODE_BINARY to the pinned OpenCode executable")
	}
	provider := &opencode.OpenCodeProvider{Binary: binary, WorkDir: t.TempDir(), StateDir: t.TempDir(), Env: []string{"PATH=" + os.Getenv("PATH")}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	config := agentctl.RunConfig{OrganizationID: "integration-org", WorkspaceID: "integration-workspace", TaskID: "integration-task", TaskSessionID: "integration-session", RunID: "integration-run", Runtime: "opencode", Agent: agentctl.AgentDefinition{Name: "Integration", Description: "Integration test", Instructions: "You are a test agent."}, Model: agentctl.ModelSelection{Provider: "anthropic", ID: "claude-sonnet-4-5"}}
	process, err := provider.Launch(ctx, config)
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
	restored, err := provider.Restore(ctx, config)
	if err != nil {
		t.Fatalf("restore real OpenCode session: %v", err)
	}
	defer func() { _ = restored.Close() }()
	if restored.SessionID() != process.SessionID() {
		t.Fatal("restoration created a replacement session")
	}
}
