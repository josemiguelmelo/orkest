package agentctl

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type chatProcess struct {
	testProcess
	calls   chan struct{}
	release chan struct{}
}

func (p *chatProcess) Prompt(ctx context.Context, _ string, o Observer) (string, error) {
	p.calls <- struct{}{}
	if err := o.Update(Update{Kind: "text", Text: "hello"}); err != nil {
		return "", err
	}
	select {
	case <-p.release:
		return "end_turn", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
func (p *chatProcess) Cancel(context.Context) error { return nil }

type chatProvider struct{ process *chatProcess }

func (p chatProvider) Runtime() string                                    { return "opencode" }
func (p chatProvider) Launch(context.Context, RunConfig) (Process, error) { return p.process, nil }

func TestJournalBeforeDeliveryAndSlowSubscriber(t *testing.T) {
	stateDir := t.TempDir()
	process := &chatProcess{testProcess: testProcess{done: make(chan struct{})}, calls: make(chan struct{}, 1), release: make(chan struct{})}
	service := NewRunControllerWithProvider(context.Background(), chatProvider{process}, stateDir)
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
	if err := service.Prompt("other", "new turn"); !errors.Is(err, ErrConversationBusy) || !errors.Is(err, ErrConversation) {
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
	var first Event
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
	service.conversation.mu.Lock()
	for range 100 {
		if err := service.conversation.recordLocked(Event{Type: "update"}); err != nil {
			t.Fatal(err)
		}
	}
	service.conversation.mu.Unlock()
	service.conversation.eventMu.Unlock()
	for range events {
	}
	close(process.release)
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
	process := &chatProcess{testProcess: testProcess{done: make(chan struct{})}, calls: make(chan struct{}, 1), release: make(chan struct{})}
	service := NewRunControllerWithProvider(context.Background(), chatProvider{process}, t.TempDir())
	defer func() { _ = service.Close() }()
	if err := service.Configure(runConfig()); err != nil {
		t.Fatal(err)
	}
	if err := service.Launch(); err != nil {
		t.Fatal(err)
	}
	service.conversation.eventMu.Lock()
	service.conversation.mu.Lock()
	err := service.conversation.openJournalLocked()
	service.conversation.mu.Unlock()
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
	if err := service.Prompt("other", "new turn"); !errors.Is(err, ErrConversationInterrupted) || !errors.Is(err, ErrConversation) {
		t.Fatalf("expected interrupted conversation category, got %v", err)
	}
	select {
	case <-process.calls:
		t.Fatal("provider invoked before durable acceptance")
	default:
	}
}
