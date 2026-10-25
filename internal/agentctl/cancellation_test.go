package agentctl

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	coreruntime "github.com/josemiguelmelo/orkest/internal/core/runtime"
)

type controlledProcess struct {
	testProcess
	observer      chan Observer
	cancelStarted chan struct{}
	cancelRelease chan struct{}
	finish        chan struct{}
	cancelError   error
}

func (p *controlledProcess) Prompt(ctx context.Context, _ string, observer Observer) (string, error) {
	p.observer <- observer
	select {
	case <-p.finish:
		return "end_turn", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (p *controlledProcess) Cancel(ctx context.Context) error {
	close(p.cancelStarted)
	select {
	case <-p.cancelRelease:
		return p.cancelError
	case <-ctx.Done():
		return ctx.Err()
	}
}

type conversationProvider struct{ process Process }

func (p conversationProvider) Runtime() coreruntime.Runtime { return coreruntime.OpenCode }
func (p conversationProvider) Launch(context.Context, RunConfig) (Process, error) {
	return p.process, nil
}

func TestCancellationAllowsStatusAndUpdates(t *testing.T) {
	p := &controlledProcess{
		testProcess: testProcess{done: make(chan struct{})},
		observer:    make(chan Observer, 1), cancelStarted: make(chan struct{}),
		cancelRelease: make(chan struct{}), finish: make(chan struct{}),
		cancelError: errors.New("delivery uncertain"),
	}
	service := NewRunControllerWithRuntimeProvider(context.Background(), conversationProvider{p}, t.TempDir())
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
		checked <- observer.Update(Update{Kind: "text", Text: "still working"})
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
	if err := <-cancelled; !errors.Is(err, ErrCancellation) {
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
		terminal = terminal || event.Type == EventPromptFailed
	}
	deadline := time.After(time.Second)
	for !terminal {
		select {
		case event := <-events:
			terminal = event.Type == EventPromptFailed
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
	if err := service.Prompt("second", "hello"); !errors.Is(err, ErrConversation) {
		t.Fatal("uncertain conversation accepted work")
	}
}

func TestPromptLimitsPreserveDeduplication(t *testing.T) {
	p := &chatProcess{testProcess: testProcess{done: make(chan struct{})}, calls: make(chan struct{}, 1), release: make(chan struct{})}
	service := NewRunControllerWithRuntimeProvider(context.Background(), chatProvider{p}, t.TempDir())
	defer func() { _ = service.Close() }()
	if err := service.Configure(runConfig()); err != nil {
		t.Fatal(err)
	}
	if err := service.Launch(); err != nil {
		t.Fatal(err)
	}
	if err := service.Prompt("too-large", strings.Repeat("x", MaxPromptBytes+1)); !errors.Is(err, ErrCommand) {
		t.Fatal("oversized prompt accepted")
	}
	if err := service.Prompt("first", "hello"); err != nil {
		t.Fatal(err)
	}
	service.conversation.mu.Lock()
	for i := 0; i < MaxCommands; i++ {
		service.conversation.commands[fmt.Sprint(i)] = sha256.Sum256([]byte("old"))
	}
	service.conversation.mu.Unlock()
	if err := service.Prompt("first", "hello"); err != nil {
		t.Fatal("duplicate rejected at capacity", err)
	}
	if err := service.Prompt("first", "changed"); !errors.Is(err, ErrCommand) {
		t.Fatal("conflicting duplicate accepted")
	}
	if err := service.Prompt("new", "hello"); !errors.Is(err, ErrCommandLimit) {
		t.Fatal("command cap ignored", err)
	}
}
