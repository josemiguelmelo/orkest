package orchestration

import (
	"context"
	"errors"

	model "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

// ExecutionGuard serializes editor transitions with all executable work delivery.
// Configure it at startup before reconnecting persisted workers.
type ExecutionGuard interface {
	BeforeExecution(context.Context, string, string, string) (func(), error)
}

func (s *LaunchService) SetExecutionGuard(guard ExecutionGuard)     { s.editorGuard = guard }
func (s *ConnectionService) SetExecutionGuard(guard ExecutionGuard) { s.editorGuard = guard }
func guardExecution(ctx context.Context, guard ExecutionGuard, org, workspace, task string) (func(), error) {
	if guard == nil {
		return func() {}, nil
	}
	return guard.BeforeExecution(ctx, org, workspace, task)
}

// EditorIdle deliberately requires terminal durable state AND confirmed process
// absence. A live conversation can independently run accepted queued commands.
func (s *LaunchService) EditorIdle(ctx context.Context, org, workspace, task string) (bool, error) {
	if s.completion == nil {
		return false, errors.New("process inspection unavailable")
	}
	launches, err := s.repo.ListTaskLaunches(ctx, org, workspace, task)
	if err != nil {
		return false, err
	}
	for _, launch := range launches {
		if launch.Status != model.LaunchCompleted && launch.Status != model.LaunchFailed {
			return false, nil
		}
		running, err := s.completion.AgentctlRunning(ctx, launch)
		if err != nil {
			return false, err
		}
		if running {
			return false, nil
		}
	}
	return true, nil
}
