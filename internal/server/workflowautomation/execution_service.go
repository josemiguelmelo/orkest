package workflowautomation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/josemiguelmelo/orkest/internal/platform/ids"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/task"
)

type ExecutionDeliveryEffects struct {
	CommandState  string
	FailureReason string
	WorkState     task.WorkState
}

// ExecutionDeliveryCoordinator persists claims and short, fenced acknowledgements.
// No callback passed to it may perform external delivery.
type ExecutionDeliveryCoordinator interface {
	ClaimExecutionCommand(context.Context, string, time.Duration) (ExecutionClaim, bool, error)
	CompleteExecutionCommand(context.Context, ExecutionClaim, func(bool) ExecutionDeliveryEffects) error
}
type ExecutionClaim struct {
	Command ExecutionCommand
	Token   string
	Attempt int32
}

var ErrStaleDeliveryClaim = errors.New("workflow delivery claim superseded")

type WorkflowExecutionService struct {
	commands   ExecutionDeliveryCoordinator
	tasks      TaskAccess
	dispatcher AgentDispatcher
	logger     *slog.Logger
}

func NewWorkflowExecutionService(commands ExecutionDeliveryCoordinator, tasks TaskAccess, dispatcher AgentDispatcher, logger *slog.Logger) *WorkflowExecutionService {
	return &WorkflowExecutionService{commands: commands, tasks: tasks, dispatcher: dispatcher, logger: logger}
}

func (s *WorkflowExecutionService) ProcessNextCommand(ctx context.Context) (bool, error) {
	token, err := ids.NewUUID()
	if err != nil {
		return false, fmt.Errorf("generate delivery claim: %w", err)
	}
	claim, found, err := s.commands.ClaimExecutionCommand(ctx, token, 30*time.Second)
	if err != nil || !found {
		return found, err
	}
	delivered := false
	reason := ""
	if claim.Attempt > 1 {
		// A previous dispatch may have failed before reservation or reached the worker.
		// Reconcile only: expiry is never permission to repeat a start.
		delivered, err = s.dispatcher.TaskCommandAccepted(ctx, claim.Command.OrganizationID, claim.Command.WorkspaceID, claim.Command.TaskID, claim.Command.ID, "")
		if !delivered {
			reason = "delivery_uncertain"
		}
	} else {
		delivered, err = s.dispatchCommand(ctx, claim.Command, claim.Token)
	}
	if err != nil {
		return true, err
	}
	err = s.commands.CompleteExecutionCommand(ctx, claim, func(failed bool) ExecutionDeliveryEffects {
		effects := ExecutionDeliveryEffects{CommandState: "failed", WorkState: task.WorkStateFailed, FailureReason: reason}
		if delivered {
			effects.CommandState = "delivered"
			effects.WorkState = task.WorkStateRunning
			if failed {
				effects.WorkState = task.WorkStateFailed
			}
		}
		return effects
	})
	if err != nil {
		return true, fmt.Errorf("acknowledge execution delivery: %w", err)
	}
	return true, nil
}

func (w *WorkflowExecutionService) dispatchCommand(ctx context.Context, c ExecutionCommand, claimToken string) (bool, error) {
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
	request := task.StartCommand{Task: value, CommandID: c.ID, DeliveryToken: claimToken, SessionMode: c.Options.SessionMode, PromptText: c.Options.PromptText}
	if err = w.dispatcher.StartTask(ctx, c.ActorID, c.OrganizationID, request); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		// Reservation is persisted before any external launch. An accepted command
		// belongs to launch recovery and must never be dispatched again.
		accepted, checkErr := w.dispatcher.TaskCommandAccepted(ctx, c.OrganizationID, c.WorkspaceID, c.TaskID, c.ID, "")
		if checkErr != nil {
			return false, checkErr
		}
		w.logger.Warn("workflow start requires attention", "workspace_id", c.WorkspaceID, "task_id", c.TaskID, "command_id", c.ID, "reason", "dispatch_failed")
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
	StartTask(context.Context, string, string, task.StartCommand) error
	TaskCommandAccepted(context.Context, string, string, string, string, string) (bool, error)
}
