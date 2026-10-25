package agentctl

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestConversationServiceDoesNotOwnProcess(t *testing.T) {
	process := &chatProcess{testProcess: testProcess{done: make(chan struct{})}, calls: make(chan struct{}, 1), release: make(chan struct{})}
	conversation := newConversationService(context.Background(), t.TempDir())
	defer func() { _ = conversation.close(); _ = process.Close() }()
	if err := conversation.Prompt("first", "hello"); !errors.Is(err, ErrConversation) {
		t.Fatal("unbound conversation accepted work")
	}
	_, events, stop, err := conversation.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	conversation.bind(runConfig(), process)
	if err := conversation.Prompt("first", "hello"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-process.calls:
	case <-time.After(time.Second):
		t.Fatal("provider was not invoked")
	}
	// Closing the component cancels and joins prompt work, persists its result,
	// and ends delivery, while process lifetime remains with the controller.
	if err := conversation.close(); err != nil {
		t.Fatal(err)
	}
	terminal := false
	for event := range events {
		terminal = terminal || event.Type == "prompt_failed"
	}
	if !terminal {
		t.Fatal("conversation closed before recording prompt termination")
	}
	select {
	case <-process.Done():
		t.Fatal("conversation closed the provider process")
	default:
	}
	if err := conversation.Prompt("second", "hello"); !errors.Is(err, ErrConversation) {
		t.Fatal("closed conversation accepted work")
	}
	if _, _, _, err := conversation.Subscribe(0); !errors.Is(err, ErrConversation) {
		t.Fatal("closed conversation accepted subscriber")
	}
}

type delayedLaunchProvider struct {
	started chan struct{}
	release chan struct{}
	process *testProcess
}

func (p delayedLaunchProvider) Runtime() string { return "opencode" }
func (p delayedLaunchProvider) Launch(context.Context, RunConfig) (Process, error) {
	close(p.started)
	<-p.release
	return p.process, nil
}

func TestControllerShutdownDuringConversationBinding(t *testing.T) {
	process := &testProcess{done: make(chan struct{})}
	provider := delayedLaunchProvider{started: make(chan struct{}), release: make(chan struct{}), process: process}
	controller := NewRunControllerWithProvider(context.Background(), provider, t.TempDir())
	defer func() { _ = controller.Close() }()
	if err := controller.Configure(runConfig()); err != nil {
		t.Fatal(err)
	}
	launched := make(chan error, 1)
	go func() { launched <- controller.Launch() }()
	<-provider.started
	closed := make(chan error, 1)
	go func() { closed <- controller.Close() }()
	// Cancellation proves shutdown fenced admission before the late launch returns.
	<-controller.ctx.Done()
	close(provider.release)
	for _, result := range []<-chan error{launched, closed} {
		select {
		case err := <-result:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("shutdown did not join the late launch")
		}
	}
	select {
	case <-process.Done():
	default:
		t.Fatal("late process was left running")
	}
	if err := controller.Prompt("late", "hello"); !errors.Is(err, ErrConversation) {
		t.Fatal("shutdown admitted a prompt")
	}
}
