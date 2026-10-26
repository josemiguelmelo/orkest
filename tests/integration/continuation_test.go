//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtime/opencode"
	"github.com/josemiguelmelo/orkest/internal/core/ids"
)

func TestOpenCodeContinuation(t *testing.T) {
	binary, modelProvider, model := os.Getenv("ORKEST_OPENCODE_BINARY"), os.Getenv("ORKEST_TEST_MODEL_PROVIDER"), os.Getenv("ORKEST_TEST_MODEL_ID")
	if binary == "" || modelProvider == "" || model == "" {
		t.Skip("set ORKEST_OPENCODE_BINARY, ORKEST_TEST_MODEL_PROVIDER, and ORKEST_TEST_MODEL_ID for real model continuation")
	}
	env := []string{"PATH=" + os.Getenv("PATH")}
	for _, name := range strings.Split(os.Getenv("ORKEST_TEST_PROVIDER_ENV"), ",") {
		if name != "" {
			value, ok := os.LookupEnv(name)
			if !ok {
				t.Fatalf("required provider variable %s is missing", name)
			}
			env = append(env, name+"="+value)
		}
	}
	provider := &opencode.OpenCodeProvider{Binary: binary, WorkDir: t.TempDir(), StateDir: t.TempDir(), Env: env}
	config := agentctl.RunConfig{OrganizationID: "test-org", WorkspaceID: "test-workspace", TaskID: "test-task", TaskSessionID: "test-session", RunID: "first", Runtime: "opencode", Agent: agentctl.AgentDefinition{Name: "Integration", Instructions: "Follow the user's instructions. Do not use tools for these memory tests."}, Model: agentctl.ModelSelection{Provider: modelProvider, ID: model}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	first, err := provider.Launch(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	token, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	observer := agentctl.Observer{Update: func(agentctl.Update) error { return nil }, Permission: func(context.Context, agentctl.Permission) (string, error) {
		return "", errors.New("unexpected permission in memory test")
	}}
	if _, err := first.(agentctl.RuntimeConversation).Prompt(ctx, "Remember this private test marker for our next turn: "+token+". Reply only acknowledged.", observer); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(provider.WorkDir, "untracked.txt")
	if err := os.WriteFile(path, []byte("preserved"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = first.Close()
	config.RunID = "second"
	restored, err := provider.Restore(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	var answer strings.Builder
	observer.Update = func(update agentctl.Update) error { answer.WriteString(update.Text); return nil }
	if _, err := restored.(agentctl.RuntimeConversation).Prompt(ctx, "What exact private test marker did I ask you to remember? Reply with only the marker.", observer); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer.String(), token) {
		t.Fatal("restored runtime did not retain conversation context")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "preserved" {
		t.Fatal("working files were not preserved", err)
	}
}
