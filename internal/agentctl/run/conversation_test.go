package run

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
)

type chatRuntimeSession struct {
	fakeRuntimeSession
	calls   chan struct{}
	release chan struct{}
}

func (p *chatRuntimeSession) Prompt(ctx context.Context, _ interaction.Command, o agentruntime.PromptObserver) (agentruntime.StopReason, error) {
	p.calls <- struct{}{}
	if err := o.Update(interaction.Update{Kind: "text", Text: "hello"}); err != nil {
		return "", err
	}
	select {
	case <-p.release:
		return agentruntime.StopReasonEndTurn, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (p *chatRuntimeSession) Cancel(context.Context) error { return nil }

type chatRuntimeHost struct{ session *chatRuntimeSession }

func (p chatRuntimeHost) Runtime() coreruntime.Runtime { return coreruntime.OpenCode }

func (p chatRuntimeHost) Launch(context.Context, configmodel.RuntimeConfig) (agentruntime.RuntimeSession, error) {
	return p.session, nil
}

func TestJournalBeforeDeliveryAndSlowSubscriber(t *testing.T) {
	stateDir := t.TempDir()
	session := &chatRuntimeSession{fakeRuntimeSession: fakeRuntimeSession{done: make(chan struct{})}, calls: make(chan struct{}, 1), release: make(chan struct{})}
	service := NewRunController(context.Background(), chatRuntimeHost{session}, stateDir, nil, nil)
	defer func() { _ = service.Close() }()
	if err := service.Configure(runConfig()); err != nil {
		t.Fatal(err)
	}
	if err := service.Launch(); err != nil {
		t.Fatal(err)
	}
	_, events, stop, err := service.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if err := service.Prompt("command", "hello"); err != nil {
		t.Fatal(err)
	}
	if err := service.Prompt("other", "new turn"); !errors.Is(err, interaction.ErrConversationBusy) || !errors.Is(err, interaction.ErrConversation) {
		t.Fatalf("expected busy conversation category, got %v", err)
	}
	select {
	case event := <-events:
		if event.Type != "prompt_accepted" {
			t.Fatal(event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing accepted")
	}
	files, err := filepath.Glob(filepath.Join(stateDir, "conversations", "*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatal("journal missing", err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var first interaction.Event
	// Decode the first JSONL record; subsequent records may already be present.
	if err := json.Unmarshal(data[:indexNewline(data)], &first); err != nil {
		t.Fatal(err)
	}
	if first.CommandID != "command" || first.Text != "hello" || first.OrganizationID != "org" {
		t.Fatalf("wrong journal record: %+v", first)
	}
	info, err := os.Stat(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatal("journal is not private")
	}
	// Fill the bounded queue without consuming it. Execution never waits on a subscriber.
	service.conversation.eventMu.Lock()
	for range 100 {
		if err := service.conversation.commitEventUnderEventLock(interaction.Event{Type: "update"}); err != nil {
			t.Fatal(err)
		}
	}
	service.conversation.eventMu.Unlock()
	for range events {
	}
	close(session.release)
}

func indexNewline(data []byte) int {
	for i, b := range data {
		if b == '\n' {
			return i
		}
	}
	return len(data)
}

func TestJournalFailurePreventsProviderPrompt(t *testing.T) {
	session := &chatRuntimeSession{fakeRuntimeSession: fakeRuntimeSession{done: make(chan struct{})}, calls: make(chan struct{}, 1), release: make(chan struct{})}
	service := NewRunController(context.Background(), chatRuntimeHost{session}, t.TempDir(), nil, nil)
	defer func() { _ = service.Close() }()
	if err := service.Configure(runConfig()); err != nil {
		t.Fatal(err)
	}
	if err := service.Launch(); err != nil {
		t.Fatal(err)
	}
	service.conversation.eventMu.Lock()
	err := service.conversation.ensureJournalUnderEventLock()
	service.conversation.eventMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	original := service.conversation.journal.file
	service.conversation.journal.file = failingFile{}
	defer func() { _ = original.Close() }()
	if err := service.Prompt("command", "hello"); err == nil {
		t.Fatal("prompt accepted with broken journal")
	}
	if service.Status().State != "interrupted" {
		t.Fatal("journal failure not visible")
	}
	if err := service.Prompt("other", "new turn"); !errors.Is(err, interaction.ErrConversationInterrupted) || !errors.Is(err, interaction.ErrConversation) {
		t.Fatalf("expected interrupted conversation category, got %v", err)
	}
	select {
	case <-session.calls:
		t.Fatal("provider invoked before durable acceptance")
	default:
	}
}

func (p chatRuntimeHost) Version() string { return "fixture-v1" }

func (p chatRuntimeHost) Restore(ctx context.Context, c configmodel.RuntimeConfig) (agentruntime.RuntimeSession, error) {
	return p.Launch(ctx, c)
}
