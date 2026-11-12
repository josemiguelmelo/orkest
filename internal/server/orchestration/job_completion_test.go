package orchestration

import (
	"context"
	"errors"
	"testing"

	runmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

type jobCompletionRepository struct {
	launchRepoFake
	owned bool
}

func (r *jobCompletionRepository) IsJobRun(context.Context, string) (bool, error) {
	return r.owned, nil
}

func TestJobCompletionClosesOnlyJobOwnedSession(t *testing.T) {
	for _, owned := range []bool{true, false} {
		t.Run(map[bool]string{true: "job", false: "legacy session"}[owned], func(t *testing.T) {
			repo := &jobCompletionRepository{owned: owned}
			controller := &sessionProcessController{}
			service, err := NewLaunchCompletionService(context.Background(), repo, controller)
			if err != nil {
				t.Fatal(err)
			}
			defer service.Close()
			launch := runmodel.Launch{RunID: "run", TaskSessionID: "session", ProcessID: "process"}
			event := runmodel.Event{RunID: "run", Payload: runmodel.AgentActivityEvent{Type: runmodel.EventPromptCompleted, TaskSessionID: "session"}}
			err = service.HandleEvent(context.Background(), launch, event)
			if owned {
				if !errors.Is(err, ErrAgentRunClosed) || controller.closed != 1 {
					t.Fatalf("job did not close: %v", err)
				}
			} else if err != nil || controller.closed != 0 {
				t.Fatalf("legacy session was closed: %v", err)
			}
		})
	}
}
