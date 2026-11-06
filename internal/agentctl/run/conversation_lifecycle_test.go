package run

import (
	"context"
	"errors"
	"testing"
	"time"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
)

func TestConversationDoesNotOwnRuntimeSession(t *testing.T) {
	session := &chatRuntimeSession{fakeRuntimeSession: fakeRuntimeSession{done: make(chan struct{})}, calls: make(chan struct{}, 1), release: make(chan struct{})}
	conversation := newConversation(context.Background(), t.TempDir(), nil)
	defer func() { _ = conversation.close(); _ = session.Close() }()
	if err := conversation.Prompt("first", "hello"); !errors.Is(err, interaction.ErrConversation) {
		t.Fatal("unbound conversation accepted work")
	}
	_, events, stop, err := conversation.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	conversation.bind(runConfig(), session)
	if err := conversation.Prompt("first", "hello"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-session.calls:
	case <-time.After(time.Second):
		t.Fatal("provider was not invoked")
	}
	// Closing the component cancels and joins prompt work, persists its result,
	// and ends delivery, while session lifetime remains with the controller.
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
	case <-session.Done():
		t.Fatal("conversation closed the provider session")
	default:
	}
	if err := conversation.Prompt("second", "hello"); !errors.Is(err, interaction.ErrConversation) {
		t.Fatal("closed conversation accepted work")
	}
	if _, _, _, err := conversation.Subscribe(0); !errors.Is(err, interaction.ErrConversation) {
		t.Fatal("closed conversation accepted subscriber")
	}
}

type delayedLaunchHost struct {
	started chan struct{}
	release chan struct{}
	session *fakeRuntimeSession
}

func (p delayedLaunchHost) Runtime() coreruntime.Runtime { return coreruntime.OpenCode }

func (p delayedLaunchHost) Launch(context.Context, configmodel.RuntimeConfig) (agentruntime.RuntimeSession, error) {
	close(p.started)
	<-p.release
	return p.session, nil
}

func TestControllerShutdownDuringConversationBinding(t *testing.T) {
	session := &fakeRuntimeSession{done: make(chan struct{})}
	provider := delayedLaunchHost{started: make(chan struct{}), release: make(chan struct{}), session: session}
	controller := NewRunController(context.Background(), provider, t.TempDir(), nil)
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
	case <-session.Done():
	default:
		t.Fatal("late session was left running")
	}
	if err := controller.Prompt("late", "hello"); !errors.Is(err, interaction.ErrConversation) {
		t.Fatal("shutdown admitted a prompt")
	}
}

func (p delayedLaunchHost) Version() string { return "fixture-v1" }

func (p delayedLaunchHost) Restore(ctx context.Context, c configmodel.RuntimeConfig) (agentruntime.RuntimeSession, error) {
	return p.Launch(ctx, c)
}
