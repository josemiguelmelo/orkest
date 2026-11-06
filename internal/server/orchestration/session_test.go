package orchestration

import (
	"context"
	"errors"
	"testing"

	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
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
	service, err := NewLaunchCompletionService(context.Background(), repo, controller)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	launch := orchestrationmodel.Launch{RunID: "run", TaskSessionID: "conversation", ProcessID: "container"}
	for _, kind := range []orchestrationmodel.EventType{orchestrationmodel.EventPromptCompleted, orchestrationmodel.EventPromptFailed, orchestrationmodel.EventQueuePaused} {
		event := orchestrationmodel.Event{RunID: "run", Payload: orchestrationmodel.AgentActivityEvent{Type: kind, TaskSessionID: "conversation"}}
		if err := service.HandleEvent(context.Background(), launch, event); err != nil {
			t.Fatal(err)
		}
	}
	if controller.closed != 0 {
		t.Fatal("prompt completion or pause stopped the session")
	}
	event := orchestrationmodel.Event{RunID: "run", Payload: orchestrationmodel.AgentActivityEvent{Type: orchestrationmodel.EventRunCompleted, TaskSessionID: "conversation"}}
	if err := service.HandleEvent(context.Background(), launch, event); !errors.Is(err, ErrAgentRunClosed) {
		t.Fatal(err)
	}
	if controller.closed != 1 {
		t.Fatal("drained run was not stopped")
	}
}
