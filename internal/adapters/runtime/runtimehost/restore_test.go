package runtimehost_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sharedacp "github.com/josemiguelmelo/orkest/internal/adapters/runtime/acp"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/opencode"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/opencode/opencodetest"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/runtimehost"
	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	"github.com/josemiguelmelo/orkest/internal/platform/sessionpath"
)

func TestRestoreSessionAcrossRuns(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	environment := runtimehost.Environment{WorkDir: t.TempDir(), StateDir: t.TempDir(), GrantedEnv: []string{"ORKEST_ACP_FIXTURE=1"}}
	config := configmodel.RunConfig{RuntimeConfig: configmodel.RuntimeConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", TaskSessionID: "conversation", RunID: "first", Runtime: "opencode", Model: configmodel.ModelSelection{Provider: "provider", ID: "model"}, Agent: configmodel.AgentDefinition{Name: "Work", Instructions: "Work"}}}
	config.MCPServers = []configmodel.MCPServer{{ID: "http", Name: "HTTP", URL: "https://example.test/mcp"}}
	home := sessionpath.Home(environment.StateDir, config.OrganizationID, config.WorkspaceID, config.TaskID, config.TaskSessionID)
	if err := os.MkdirAll(filepath.Join(home, ".aws"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".aws", "config"), []byte("fixture-home-config"), 0o600); err != nil {
		t.Fatal(err)
	}
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
	config.MCPServers[0].Transport = "http"
	environment.GrantedEnv = append(environment.GrantedEnv, "ORKEST_GIT_AUTHOR_NAME=Restored Agent", "ORKEST_GIT_AUTHOR_EMAIL=restored@example.com")
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
	text = ""
	if _, err := runtime.Prompt(ctx, interaction.Command{ID: "home-command", Text: "probe:home-config"}, agentruntime.PromptObserver{Update: func(update interaction.Update) error { text += update.Text; return nil }}); err != nil {
		t.Fatal(err)
	}
	if text != "fixture-home-configHello from fixture" {
		t.Fatal("restored runtime did not read the home-relative configuration")
	}
	text = ""
	if _, err := runtime.Prompt(ctx, interaction.Command{ID: "identity-command", Text: "probe:git-identity"}, agentruntime.PromptObserver{Update: func(update interaction.Update) error { text += update.Text; return nil }}); err != nil {
		t.Fatal(err)
	}
	if text != "Restored Agent|restored@example.com|Restored Agent|restored@example.comHello from fixture" {
		t.Fatalf("restored runtime did not receive configured Git identity: %q", text)
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
	config := configmodel.RunConfig{RuntimeConfig: configmodel.RuntimeConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", TaskSessionID: "conversation", RunID: "run", Runtime: "opencode", Model: configmodel.ModelSelection{Provider: "provider", ID: "model"}, Agent: configmodel.AgentDefinition{Name: "Work", Instructions: "Work"}}}
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
	config := configmodel.RuntimeConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", TaskSessionID: "conversation", RunID: "run", Runtime: "opencode", Model: configmodel.ModelSelection{Provider: "provider", ID: "model"}, Agent: configmodel.AgentDefinition{Name: "Work", Instructions: "Work"}}
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

// Restore an actual pre-refactor manifest, then exercise the next message. Merely
// decoding the manifest cannot establish continuation compatibility.
func TestV1ManifestRestoreAndFollowUp(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	environment := runtimehost.Environment{WorkDir: t.TempDir(), StateDir: t.TempDir(), GrantedEnv: []string{"ORKEST_ACP_FIXTURE=1"}}
	config := configmodel.RuntimeConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", TaskSessionID: "session", RunID: "revision-2", Runtime: "opencode", Agent: configmodel.AgentDefinition{Name: "Work", Description: "Work", Instructions: "Work"}, Model: configmodel.ModelSelection{Provider: "provider", ID: "model"}}
	fixture, err := os.ReadFile("testdata/session-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err = json.Unmarshal(fixture, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["workDir"] = environment.WorkDir
	manifest["sessionId"] = "fixture-session"
	fixture, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(environment.StateDir, "961e0f4564a3db8eeae39c31bea4d9026bb33bc064377d6252b33e63ee4224e4", "sessions", "3f3af1ecebbd1410ab417ec0d27bbfcb5d340e177ae159b59fc8626c2dfd9175")
	if err = os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "session.json"), fixture, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := opencodetest.NewHost(binary, environment).Restore(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	var text string
	if _, err = session.Prompt(ctx, interaction.Command{ID: "legacy-follow-up", Text: "hello"}, agentruntime.PromptObserver{Update: func(update interaction.Update) error { text += update.Text; return nil }}); err != nil {
		t.Fatal(err)
	}
	if text != "Hello from fixture" {
		t.Fatalf("follow-up failed: %q", text)
	}
}

func TestUnconfirmedModelPreservesSessionAndAllowsCompatibleFollowUp(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	environment := runtimehost.Environment{WorkDir: t.TempDir(), StateDir: t.TempDir(), GrantedEnv: []string{"ORKEST_ACP_FIXTURE=1"}}
	config := configmodel.RuntimeConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", TaskSessionID: "session", RunID: "first", Runtime: "opencode", Agent: configmodel.AgentDefinition{Name: "Work", Instructions: "Work"}, Model: configmodel.ModelSelection{Provider: "provider", ID: "model"}}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	first, err := opencodetest.NewHost(binary, environment).Launch(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(environment.StateDir, "*", "sessions", "*", "session.json"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("manifest paths: %v %v", paths, err)
	}
	before, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	environment.GrantedEnv = append(environment.GrantedEnv, "ORKEST_ACP_UNCONFIRMED_MODEL=1")
	config.RunID = "follow-up"
	if _, err = opencodetest.NewHost(binary, environment).Restore(ctx, config); err == nil || !strings.Contains(err.Error(), "configured model") {
		t.Fatalf("unconfirmed model restored: %v", err)
	}
	after, err := os.ReadFile(paths[0])
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed confirmation changed saved session", err)
	}
	environment.GrantedEnv = environment.GrantedEnv[:1]
	restored, err := opencodetest.NewHost(binary, environment).Restore(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	if restored.SessionID() != first.SessionID() {
		t.Fatal("session replaced after confirmation failure")
	}
	if _, err = restored.Prompt(ctx, interaction.Command{ID: "confirmed-follow-up", Text: "hello"}, agentruntime.PromptObserver{}); err != nil {
		t.Fatal(err)
	}
}
