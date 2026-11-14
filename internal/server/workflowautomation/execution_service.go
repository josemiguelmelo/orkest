package workflowautomation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
)

type ExecutionDeliveryEffects struct {
	CommandState  string
	FailureReason string
	WorkState     taskmodel.WorkState
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
		delivered, reason, err = s.dispatchCommand(ctx, claim.Command, claim.Token)
	}
	if err != nil {
		return true, err
	}
	err = s.commands.CompleteExecutionCommand(ctx, claim, func(failed bool) ExecutionDeliveryEffects {
		effects := ExecutionDeliveryEffects{CommandState: "failed", WorkState: taskmodel.WorkStateFailed, FailureReason: reason}
		if delivered {
			effects.CommandState = "delivered"
			effects.WorkState = taskmodel.WorkStateRunning
			if failed {
				effects.WorkState = taskmodel.WorkStateFailed
			}
		}
		return effects
	})
	if err != nil {
		return true, fmt.Errorf("acknowledge execution delivery: %w", err)
	}
	return true, nil
}

func (w *WorkflowExecutionService) dispatchCommand(ctx context.Context, c ExecutionCommand, claimToken string) (bool, string, error) {
	accepted, err := w.dispatcher.TaskCommandAccepted(ctx, c.OrganizationID, c.WorkspaceID, c.TaskID, c.ID, "")
	if err != nil {
		return false, "", err
	}
	if accepted {
		return true, "", nil
	}
	if err = w.tasks.AuthorizeExecution(ctx, c.ActorID, c.OrganizationID, c.WorkspaceID, c.TaskID, true); err != nil {
		if errors.Is(err, modelerrors.ErrForbidden) || errors.Is(err, modelerrors.ErrNotFound) {
			return false, "authorization_revoked", nil
		}
		return false, "", err
	}
	value, err := w.tasks.Get(ctx, c.ActorID, c.OrganizationID, c.WorkspaceID, c.TaskID)
	if err != nil {
		return false, "", err
	}
	if value.StageVersion != c.StageVersion {
		return false, "stage_changed", nil
	}
	if c.Options.GuardAssignment && value.AssigneeAgentID != c.Options.ExpectedAssigneeID {
		return false, "assignment_changed", nil
	}
	request := taskmodel.StartCommand{Task: value, CommandID: c.ID, DeliveryToken: claimToken, SessionMode: c.Options.SessionMode, PromptText: c.Options.PromptText, ExpectedAssigneeID: c.Options.ExpectedAssigneeID, GuardAssignment: c.Options.GuardAssignment, Integration: c.Options.Integration}
	if err = w.dispatcher.StartTask(ctx, c.ActorID, c.OrganizationID, request); err != nil {
		if ctx.Err() != nil {
			return false, "", ctx.Err()
		}
		// Reservation is persisted before external launch. Accepted commands belong
		// to launch recovery; a dispatch error cannot authorize a second execution.
		accepted, checkErr := w.dispatcher.TaskCommandAccepted(ctx, c.OrganizationID, c.WorkspaceID, c.TaskID, c.ID, "")
		if checkErr != nil {
			return false, "", checkErr
		}
		reason := "dispatch_failed"
		if errors.Is(err, taskmodel.ErrIntegrationSession) {
			reason = "session_incompatible"
		}
		if accepted {
			return true, "", nil
		}
		if w.logger != nil {
			failureStage, causeType := dispatchFailureDetails(err)
			w.logger.Warn("workflow start requires attention",
				"workspace_id", c.WorkspaceID,
				"task_id", c.TaskID,
				"command_id", c.ID,
				"stage_version", c.StageVersion,
				"session_mode", c.Options.SessionMode,
				"reason", reason,
				"failure_stage", failureStage,
				"cause_type", causeType,
			)
		}
		return false, reason, nil
	}
	return true, "", nil
}

func dispatchFailureDetails(err error) (string, string) {
	stage := "task_dispatch"
	cause := err
	var failure *taskmodel.StartFailure
	if errors.As(err, &failure) {
		stage = failure.Stage
		cause = failure.Cause
	}
	for {
		unwrapped := errors.Unwrap(cause)
		if unwrapped == nil {
			break
		}
		cause = unwrapped
	}
	return stage, fmt.Sprintf("%T", cause)
}

// TaskAccess supplies current task state and checks execution authorization.
type TaskAccess interface {
	Get(context.Context, string, string, string, string) (taskmodel.Task, error)
	AuthorizeExecution(context.Context, string, string, string, string, bool) error
}

type AgentDispatcher interface {
	StartTask(context.Context, string, string, taskmodel.StartCommand) error
	TaskCommandAccepted(context.Context, string, string, string, string, string) (bool, error)
}
