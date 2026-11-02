package task

import (
	"context"
	"errors"
	"log/slog"
	"time"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
)

type WorkflowCommand struct {
	ID, OrganizationID, WorkspaceID, TaskID, ActorID string
	StageVersion                                     int64
	Options                                          StartWorkCommand
}

// StartWorkCommand carries execution options only; task transitions and domain
// events have separate payloads.
type StartWorkCommand struct {
	Action       string `json:"action"`
	CommandID    string `json:"commandId"`
	StageVersion int64  `json:"stageVersion"`
	SessionMode  string `json:"sessionMode"`
	PromptText   string `json:"promptText"`
}

type EventProcessor interface {
	ProcessWorkflowEvent(context.Context) (bool, error)
}

type CommandRepository interface {
	// DeliverWorkflowCommand holds an exclusive command lock through delivery.
	// Cancellation rolls back delivery intent; the next worker reconciles by ID.
	DeliverWorkflowCommand(context.Context, func(context.Context, WorkflowCommand) (bool, error)) (bool, error)
}

type WorkflowWorker struct {
	commands   CommandRepository
	tasks      *Service
	events     EventProcessor
	dispatcher TaskDispatcher
	logger     *slog.Logger
}

func NewWorkflowWorker(commands CommandRepository, tasks *Service, dispatcher TaskDispatcher, logger *slog.Logger) *WorkflowWorker {
	return &WorkflowWorker{commands: commands, tasks: tasks, events: tasks, dispatcher: dispatcher, logger: logger}
}

func (w *WorkflowWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		for ctx.Err() == nil {
			found, err := w.ProcessOne(ctx)
			if err != nil {
				if ctx.Err() == nil {
					w.logger.Error("workflow command processing failed")
				}
				break
			}
			if !found {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ProcessOne processes one event and delivers one command. It is also useful for
// deterministic recovery tests and bounded worker shutdown.
func (w *WorkflowWorker) ProcessOne(ctx context.Context) (bool, error) {
	processed, err := w.events.ProcessWorkflowEvent(ctx)
	if err != nil {
		return false, err
	}
	delivered, err := w.commands.DeliverWorkflowCommand(ctx, w.deliver)
	return processed || delivered, err
}

func (w *WorkflowWorker) deliver(ctx context.Context, c WorkflowCommand) (bool, error) {
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
	if value.SessionMode == "" {
		value.SessionMode = "new"
	}
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
		w.logger.Warn("workflow start requires attention", "workspace_id", c.WorkspaceID, "task_id", c.TaskID, "command_id", c.ID)
		return accepted, nil
	}
	return true, nil
}
