//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
	"github.com/josemiguelmelo/orkest/internal/agentruntime/opencode"
)

// TestOpenCodeHandshake needs the pinned executable but makes no model request.
func TestOpenCodeHandshake(t *testing.T) {
	binary := os.Getenv("ORKEST_OPENCODE_BINARY")
	if binary == "" {
		t.Fatal("set ORKEST_OPENCODE_BINARY to the pinned OpenCode executable")
	}
	provider := &opencode.Provider{Binary: binary, WorkDir: t.TempDir(), StateDir: t.TempDir(), Env: []string{"PATH=" + os.Getenv("PATH")}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	process, err := provider.Launch(ctx, agentctl.Configuration{OrganizationID: "integration-org", WorkspaceID: "integration-workspace", TaskID: "integration-task", RunID: "integration-run", Runtime: "opencode", Model: agentctl.Model{Provider: "anthropic", ID: "claude-sonnet-4-5"}, SystemPrompt: "You are a test agent."})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = process.Close() }()
	if process.SessionID() == "" {
		t.Fatal("missing session identity")
	}
}
