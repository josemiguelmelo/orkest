package orchestration

import (
	"context"
	"errors"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
)

type sessionProcessController struct{ closed int }

func (*sessionProcessController) AgentctlRunning(context.Context, string) (bool, error) {
	return true, nil
}

func (*sessionProcessController) AgentctlRunningByRunID(context.Context, string) (bool, error) {
	return true, nil
}

func (c *sessionProcessController) TerminateAgentctl(context.Context, string) error {
	c.closed++
	return nil
}

func (c *sessionProcessController) TerminateAgentctlByRunID(context.Context, string) error {
	c.closed++
	return nil
}

func TestSessionCompletionWaitsForQueueDrain(t *testing.T) {
	repo := &launchRepoFake{}
	controller := &sessionProcessController{}
	service, err := NewLaunchCompletionService(repo, controller)
	if err != nil {
		t.Fatal(err)
	}
	launch := Launch{RunID: "run", TaskSessionID: "conversation", ProcessID: "container"}
	for _, kind := range []interaction.EventType{interaction.EventPromptCompleted, interaction.EventPromptFailed, interaction.EventQueuePaused} {
		event := Event{RunID: "run", Payload: interaction.Event{Type: kind, TaskSessionID: "conversation"}}
		if err := service.HandleEvent(context.Background(), launch, event); err != nil {
			t.Fatal(err)
		}
	}
	if controller.closed != 0 {
		t.Fatal("prompt completion or pause stopped the session")
	}
	event := Event{RunID: "run", Payload: interaction.Event{Type: interaction.EventRunCompleted, TaskSessionID: "conversation"}}
	if err := service.HandleEvent(context.Background(), launch, event); !errors.Is(err, ErrAgentRunClosed) {
		t.Fatal(err)
	}
	if controller.closed != 1 {
		t.Fatal("drained run was not stopped")
	}
}
