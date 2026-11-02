package workflowautomation

import (
	"context"
	"errors"
	"log/slog"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/task"
)

type ExecutionDeliveryEffects struct {
	CommandState string
	WorkState    task.WorkState
}

type ExecutionCommandRepository interface {
	// DeliverExecutionCommand retains the exclusive delivery lock until effects commit.
	DeliverExecutionCommand(context.Context, func(context.Context, ExecutionCommand, ExecutionHealthReader) (ExecutionDeliveryEffects, error)) (bool, error)
}

type ExecutionHealthReader interface {
	TaskExecutionFailed(context.Context) (bool, error)
}

type WorkflowExecutionService struct {
	commands   ExecutionCommandRepository
	tasks      TaskAccess
	dispatcher AgentDispatcher
	logger     *slog.Logger
}

func NewWorkflowExecutionService(commands ExecutionCommandRepository, tasks TaskAccess, dispatcher AgentDispatcher, logger *slog.Logger) *WorkflowExecutionService {
	return &WorkflowExecutionService{commands: commands, tasks: tasks, dispatcher: dispatcher, logger: logger}
}

func (s *WorkflowExecutionService) ProcessNextCommand(ctx context.Context) (bool, error) {
	return s.commands.DeliverExecutionCommand(ctx, s.deliverCommand)
}

func (s *WorkflowExecutionService) deliverCommand(ctx context.Context, command ExecutionCommand, health ExecutionHealthReader) (ExecutionDeliveryEffects, error) {
	delivered, err := s.dispatchCommand(ctx, command)
	if err != nil {
		return ExecutionDeliveryEffects{}, err
	}
	effects := ExecutionDeliveryEffects{CommandState: "failed", WorkState: task.WorkStateFailed}
	if delivered {
		effects.CommandState, effects.WorkState = "delivered", task.WorkStateRunning
		failed, err := health.TaskExecutionFailed(ctx)
		if err != nil {
			return ExecutionDeliveryEffects{}, err
		}
		if failed {
			effects.WorkState = task.WorkStateFailed
		}
	}
	return effects, nil
}

func (w *WorkflowExecutionService) dispatchCommand(ctx context.Context, c ExecutionCommand) (bool, error) {
	accepted, err := w.dispatcher.TaskCommandAccepted(ctx, c.OrganizationID, c.WorkspaceID, c.TaskID, c.ID, "")
	if err != nil {
		return false, err
	}
	if accepted {
		return true, nil
	}
	if err = w.tasks.AuthorizeExecution(ctx, c.ActorID, c.OrganizationID, c.WorkspaceID, c.TaskID, true); err != nil {
		if errors.Is(err, modelerrors.ErrForbidden) || errors.Is(err, modelerrors.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	value, err := w.tasks.Get(ctx, c.ActorID, c.OrganizationID, c.WorkspaceID, c.TaskID)
	if err != nil {
		return false, err
	}
	if value.StageVersion != c.StageVersion {
		return false, nil
	}
	value.CommandID, value.SessionMode, value.PromptText = c.ID, c.Options.SessionMode, c.Options.PromptText
	if err = w.dispatcher.StartTask(ctx, c.ActorID, c.OrganizationID, value); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		// Reservation is persisted before any external launch. An accepted command
		// belongs to launch recovery and must never be dispatched again.
		accepted, checkErr := w.dispatcher.TaskCommandAccepted(ctx, c.OrganizationID, c.WorkspaceID, c.TaskID, c.ID, "")
		if checkErr != nil {
			return false, checkErr
		}
		w.logger.Warn("workflow start requires attention", "workspace_id", c.WorkspaceID, "task_id", c.TaskID, "command_id", c.ID, "reason", err)
		return accepted, nil
	}
	return true, nil
}

// TaskAccess supplies current task state and checks execution authorization.
type TaskAccess interface {
	Get(context.Context, string, string, string, string) (task.Task, error)
	AuthorizeExecution(context.Context, string, string, string, string, bool) error
}

type AgentDispatcher interface {
	StartTask(context.Context, string, string, task.Task) error
	TaskCommandAccepted(context.Context, string, string, string, string, string) (bool, error)
}
