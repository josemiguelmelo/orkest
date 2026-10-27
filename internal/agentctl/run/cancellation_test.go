package run

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	coreruntime "github.com/josemiguelmelo/orkest/internal/core/runtime"
)

type controlledRuntimeSession struct {
	fakeRuntimeSession
	observer      chan agentruntime.PromptObserver
	cancelStarted chan struct{}
	cancelRelease chan struct{}
	finish        chan struct{}
	cancelError   error
}

func (p *controlledRuntimeSession) Prompt(ctx context.Context, _ interaction.Command, observer agentruntime.PromptObserver) (agentruntime.StopReason, error) {
	p.observer <- observer
	select {
	case <-p.finish:
		return agentruntime.StopReasonEndTurn, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (p *controlledRuntimeSession) Cancel(ctx context.Context) error {
	close(p.cancelStarted)
	select {
	case <-p.cancelRelease:
		return p.cancelError
	case <-ctx.Done():
		return ctx.Err()
	}
}

type conversationRuntimeHost struct{ process agentruntime.RuntimeSession }

func (p conversationRuntimeHost) Runtime() coreruntime.Runtime { return coreruntime.OpenCode }
func (p conversationRuntimeHost) Launch(context.Context, runconfig.RuntimeConfig) (agentruntime.RuntimeSession, error) {
	return p.process, nil
}

func TestCancellationAllowsStatusAndUpdates(t *testing.T) {
	p := &controlledRuntimeSession{
		fakeRuntimeSession: fakeRuntimeSession{done: make(chan struct{})},
		observer:           make(chan agentruntime.PromptObserver, 1), cancelStarted: make(chan struct{}),
		cancelRelease: make(chan struct{}), finish: make(chan struct{}),
		cancelError: errors.New("delivery uncertain"),
	}
	service := NewRunController(context.Background(), conversationRuntimeHost{p}, t.TempDir(), nil)
	defer func() { _ = service.Close() }()
	if err := service.Configure(runConfig()); err != nil {
		t.Fatal(err)
	}
	if err := service.Launch(); err != nil {
		t.Fatal(err)
	}
	if err := service.Prompt("first", "hello"); err != nil {
		t.Fatal(err)
	}
	observer := <-p.observer
	cancelled := make(chan error, 1)
	go func() { cancelled <- service.CancelPrompt("first") }()
	<-p.cancelStarted
	checked := make(chan error, 1)
	go func() {
		_ = service.Status()
		checked <- observer.Update(interaction.Update{Kind: "text", Text: "still working"})
	}()
	select {
	case err := <-checked:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("provider cancellation blocked status or updates")
	}
	close(p.cancelRelease)
	if err := <-cancelled; !errors.Is(err, interaction.ErrCancellation) {
		t.Fatalf("cancel: %v", err)
	}
	replay, events, stop, err := service.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	close(p.finish)
	terminal := false
	for _, event := range replay {
		terminal = terminal || event.Type == interaction.EventPromptFailed
	}
	deadline := time.After(time.Second)
	for !terminal {
		select {
		case event := <-events:
			terminal = event.Type == interaction.EventPromptFailed
		case <-deadline:
			t.Fatal("missing terminal event")
		}
	}
	if status := service.Status(); status.State != StateInterrupted || !status.ProcessAlive {
		t.Fatalf("completion erased uncertain cancellation: %+v", status)
	}
	_ = p.Close()
	if status := service.Status(); status.State != StateInterrupted || status.ProcessAlive {
		t.Fatalf("exit erased interruption: %+v", status)
	}
	if err := service.Prompt("second", "hello"); !errors.Is(err, interaction.ErrConversation) {
		t.Fatal("uncertain conversation accepted work")
	}
}

func TestPromptLimitsPreserveDeduplication(t *testing.T) {
	p := &chatRuntimeSession{fakeRuntimeSession: fakeRuntimeSession{done: make(chan struct{})}, calls: make(chan struct{}, 1), release: make(chan struct{})}
	service := NewRunController(context.Background(), chatRuntimeHost{p}, t.TempDir(), nil)
	defer func() { _ = service.Close() }()
	if err := service.Configure(runConfig()); err != nil {
		t.Fatal(err)
	}
	if err := service.Launch(); err != nil {
		t.Fatal(err)
	}
	if err := service.Prompt("too-large", strings.Repeat("x", interaction.MaxPromptBytes+1)); !errors.Is(err, interaction.ErrCommand) {
		t.Fatal("oversized prompt accepted")
	}
	if err := service.Prompt("first", "hello"); err != nil {
		t.Fatal(err)
	}
	service.conversation.mu.Lock()
	for i := 0; i < interaction.MaxCommands; i++ {
		service.conversation.state.commands[fmt.Sprint(i)] = commandRecord{contentHash: sha256.Sum256([]byte("old")), state: interaction.CommandCompleted}
	}
	service.conversation.mu.Unlock()
	if err := service.Prompt("first", "hello"); err != nil {
		t.Fatal("duplicate rejected at capacity", err)
	}
	if err := service.Prompt("first", "changed"); !errors.Is(err, interaction.ErrCommand) {
		t.Fatal("conflicting duplicate accepted")
	}
	if err := service.Prompt("new", "hello"); !errors.Is(err, interaction.ErrCommandLimit) {
		t.Fatal("command cap ignored", err)
	}
}

func (p conversationRuntimeHost) Version() string { return "fixture-v1" }
func (p conversationRuntimeHost) Restore(ctx context.Context, c runconfig.RuntimeConfig) (agentruntime.RuntimeSession, error) {
	return p.Launch(ctx, c)
}
