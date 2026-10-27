//go:build integration

package integration

import (
"context"
"errors"
"github.com/josemiguelmelo/orkest/internal/agentctl/runtime/opencode"
"github.com/josemiguelmelo/orkest/internal/agentctl/runtimehost"
"github.com/josemiguelmelo/orkest/internal/core/ids"
"os"
"path/filepath"
"strings"
"testing"
"time"
agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
sharedacp "github.com/josemiguelmelo/orkest/internal/agentctl/runtime/acp"
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
	workDir, stateDir := t.TempDir(), t.TempDir()
	host := runtimehost.New(opencode.Profile{Binary: binary}, sharedacp.Factory{}, runtimehost.Environment{WorkDir: workDir, StateDir: stateDir, GrantedEnv: env})
	config := runconfig.RunConfig{RuntimeConfig: runconfig.RuntimeConfig{OrganizationID: "test-org", WorkspaceID: "test-workspace", TaskID: "test-task", TaskSessionID: "test-session", RunID: "first", Runtime: "opencode", Agent: runconfig.AgentDefinition{Name: "Integration", Instructions: "Follow the user's instructions. Do not use tools for these memory tests."}, Model: runconfig.ModelSelection{Provider: modelProvider, ID: model}}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	first, err := host.Launch(ctx, config.ForRuntime())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	token, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	observer := agentruntime.PromptObserver{Update: func(interaction.Update) error { return nil }, Permission: func(context.Context, interaction.Permission) (string, error) {
		return "", errors.New("unexpected permission in memory test")
	}}
	if _, err := first.Prompt(ctx, interaction.Command{ID: "remember", Text: "Remember this private test marker for our next turn: " + token + ". Reply only acknowledged."}, observer); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workDir, "untracked.txt")
	if err := os.WriteFile(path, []byte("preserved"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = first.Close()
	config.RunID = "second"
	restored, err := host.Restore(ctx, config.ForRuntime())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	var answer strings.Builder
	observer.Update = func(update interaction.Update) error { answer.WriteString(update.Text); return nil }
	if _, err := restored.Prompt(ctx, interaction.Command{ID: "test-command", Text: "What exact private test marker did I ask you to remember? Reply with only the marker."}, observer); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer.String(), token) {
		t.Fatal("restored runtime did not retain conversation context")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "preserved" {
		t.Fatal("working files were not preserved", err)
	}
}
