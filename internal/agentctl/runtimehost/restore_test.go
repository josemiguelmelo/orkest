package runtimehost_test

import (
	"context"
	"os"
	"testing"
	"time"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	sharedacp "github.com/josemiguelmelo/orkest/internal/agentctl/runtime/acp"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtime/opencode"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtime/opencode/opencodetest"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtimehost"
)

func TestRestoreSessionAcrossRuns(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	environment := runtimehost.Environment{WorkDir: t.TempDir(), StateDir: t.TempDir(), GrantedEnv: []string{"ORKEST_ACP_FIXTURE=1"}}
	config := runconfig.RunConfig{RuntimeConfig: runconfig.RuntimeConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", TaskSessionID: "conversation", RunID: "first", Runtime: "opencode", Model: runconfig.ModelSelection{Provider: "provider", ID: "model"}, Agent: runconfig.AgentDefinition{Name: "Work", Instructions: "Work"}}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	first, err := opencodetest.NewHost(binary, environment).Launch(ctx, config.ForRuntime())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := opencodetest.NewHost(binary, environment).Restore(ctx, config.ForRuntime()); err == nil {
		t.Fatal("restored while previous owner was alive")
	}
	_ = first.Close()
	config.RunID = "second"
	restored, err := opencodetest.NewHost(binary, environment).Restore(ctx, config.ForRuntime())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	if restored.SessionID() != first.SessionID() {
		t.Fatal("session identity changed")
	}
	var text string
	runtime := restored
	if _, err := runtime.Prompt(ctx, interaction.Command{ID: "test-command", Text: "hello"}, agentruntime.PromptObserver{Update: func(update interaction.Update) error { text += update.Text; return nil }}); err != nil {
		t.Fatal(err)
	}
	if text != "Hello from fixture" {
		t.Fatal("history replay leaked into live conversation", text)
	}
	_ = restored.Close()
	config.Model.ID = "changed"
	if _, err := opencodetest.NewHost(binary, environment).Restore(ctx, config.ForRuntime()); err == nil {
		t.Fatal("changed configuration restored silently")
	}
}

func TestRestoreUnavailableNeverCreatesSession(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	environment := runtimehost.Environment{WorkDir: t.TempDir(), StateDir: t.TempDir(), GrantedEnv: []string{"ORKEST_ACP_FIXTURE=1"}}
	config := runconfig.RunConfig{RuntimeConfig: runconfig.RuntimeConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", TaskSessionID: "conversation", RunID: "run", Runtime: "opencode", Model: runconfig.ModelSelection{Provider: "provider", ID: "model"}, Agent: runconfig.AgentDefinition{Name: "Work", Instructions: "Work"}}}
	if _, err := opencodetest.NewHost(binary, environment).Restore(context.Background(), config.ForRuntime()); err == nil {
		t.Fatal("missing session restored")
	}
	process, err := opencodetest.NewHost(binary, environment).Launch(context.Background(), config.ForRuntime())
	if err != nil {
		t.Fatal(err)
	}
	_ = process.Close()
	environment.GrantedEnv = append(environment.GrantedEnv, "ORKEST_ACP_NO_RESTORE=1")
	if _, err := opencodetest.NewHost(binary, environment).Restore(context.Background(), config.ForRuntime()); err == nil {
		t.Fatal("unsupported restoration accepted")
	}
}

// Only this deterministic fixture attests complete history coverage. Production
// OpenCode keeps its unqualified coverage and conservative recovery policy.
type verifiedHistoryProfile struct{ opencode.Profile }

func (p verifiedHistoryProfile) Descriptor() agentruntime.RuntimeDescriptor {
	descriptor := p.Profile.Descriptor()
	descriptor.ReplayCoverageVerified = true
	return descriptor
}

func TestRestoreCollectsAndSealsHistoryBeforeReconciliation(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	host := runtimehost.New(verifiedHistoryProfile{opencode.Profile{Binary: binary}}, sharedacp.Factory{}, runtimehost.Environment{
		WorkDir: t.TempDir(), StateDir: t.TempDir(), GrantedEnv: []string{"ORKEST_ACP_FIXTURE=1"},
	})
	config := runconfig.RuntimeConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", TaskSessionID: "conversation", RunID: "run", Runtime: "opencode", Model: runconfig.ModelSelection{Provider: "provider", ID: "model"}, Agent: runconfig.AgentDefinition{Name: "Work", Instructions: "Work"}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	first, err := host.Launch(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	command := interaction.Command{ID: "checkpoint", Text: "hello"}
	if _, err := first.Prompt(ctx, command, agentruntime.PromptObserver{}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := host.Restore(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	check := func(command interaction.Command, want agentruntime.RecoveryAction) {
		t.Helper()
		decision, err := restored.Reconcile(ctx, command)
		if err != nil || decision.Action != want {
			t.Fatalf("unexpected reconciliation: %+v %v", decision, err)
		}
	}
	check(command, agentruntime.RecoveryContinue)
	live := interaction.Command{ID: "live", Text: "hello again"}
	if _, err := restored.Prompt(ctx, live, agentruntime.PromptObserver{}); err != nil {
		t.Fatal(err)
	}
	check(command, agentruntime.RecoveryContinue)
	check(live, agentruntime.RecoveryPause)
}
