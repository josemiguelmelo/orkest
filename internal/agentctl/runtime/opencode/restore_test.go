package opencode

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

func TestRestoreSessionAcrossRuns(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	provider := &OpenCodeProvider{Binary: binary, WorkDir: t.TempDir(), StateDir: t.TempDir(), Env: []string{"ORKEST_ACP_FIXTURE=1"}}
	config := agentctl.RunConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", TaskSessionID: "conversation", RunID: "first", Runtime: "opencode", Model: agentctl.ModelSelection{Provider: "provider", ID: "model"}, Agent: agentctl.AgentDefinition{Name: "Work", Instructions: "Work"}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	first, err := provider.Launch(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Restore(ctx, config); err == nil {
		t.Fatal("restored while previous owner was alive")
	}
	_ = first.Close()
	config.RunID = "second"
	restored, err := provider.Restore(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	if restored.SessionID() != first.SessionID() {
		t.Fatal("session identity changed")
	}
	var text string
	runtime := restored.(agentctl.RuntimeConversation)
	if _, err := runtime.Prompt(ctx, "hello", agentctl.Observer{Update: func(update agentctl.Update) error { text += update.Text; return nil }}); err != nil {
		t.Fatal(err)
	}
	if text != "Hello from fixture" {
		t.Fatal("history replay leaked into live conversation", text)
	}
	_ = restored.Close()
	config.Model.ID = "changed"
	if _, err := provider.Restore(ctx, config); err == nil {
		t.Fatal("changed configuration restored silently")
	}
}

func TestRestoreUnavailableNeverCreatesSession(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	provider := &OpenCodeProvider{Binary: binary, WorkDir: t.TempDir(), StateDir: t.TempDir(), Env: []string{"ORKEST_ACP_FIXTURE=1"}}
	config := agentctl.RunConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", TaskSessionID: "conversation", RunID: "run", Runtime: "opencode", Model: agentctl.ModelSelection{Provider: "provider", ID: "model"}, Agent: agentctl.AgentDefinition{Name: "Work", Instructions: "Work"}}
	if _, err := provider.Restore(context.Background(), config); err == nil {
		t.Fatal("missing session restored")
	}
	process, err := provider.Launch(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	_ = process.Close()
	provider.Env = append(provider.Env, "ORKEST_ACP_NO_RESTORE=1")
	if _, err := provider.Restore(context.Background(), config); err == nil {
		t.Fatal("unsupported restoration accepted")
	}
}

func TestRecoveryEvidenceSeparatesIdenticalUserTurns(t *testing.T) {
	process := &process{client: &runtimeClient{replayUser: wirePrompt("first", "continue")}}
	if !process.CanContinue("first", "continue") {
		t.Fatal("matching checkpoint was not recognized")
	}
	if process.CanContinue("second", "continue") {
		t.Fatal("an older identical message was mistaken for the interrupted command")
	}
	process.client.replayTools = true
	if process.CanContinue("first", "continue") {
		t.Fatal("tool effects were ignored during reconciliation")
	}
}
