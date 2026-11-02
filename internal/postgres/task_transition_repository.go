package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"

	workflowautomation "github.com/josemiguelmelo/orkest/internal/workflowautomation"

	"github.com/josemiguelmelo/orkest/internal/core/ids"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/task"
)

func enqueueStageStart(ctx context.Context, q *sqlc.Queries, workspaceID, taskID, actorID string, version int64, in workflowautomation.TransitionRequest) error {
	if in.CommandID == "" {
		id, err := ids.NewUUID()
		if err != nil {
			return err
		}
		in.CommandID = id
	}
	options, err := json.Marshal(workflowautomation.StartWorkOptions{Action: "start", CommandID: in.CommandID, StageVersion: in.StageVersion, SessionMode: in.SessionMode, PromptText: in.PromptText})
	if err != nil {
		return err
	}
	if err = q.EnqueueWorkflowCommand(ctx, sqlc.EnqueueWorkflowCommandParams{ID: in.CommandID, WorkspaceID: workspaceID, TaskID: taskID, StageVersion: version, ActorID: actorID, Options: options}); err != nil {
		return mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
	}
	return q.SetStageWorkState(ctx, sqlc.SetStageWorkStateParams{WorkspaceID: workspaceID, ID: taskID, StageVersion: version, StageWorkState: string(task.WorkStateQueued)})
}

// WithTaskTransition loads locked facts and persists the task policy's effects.
// Command reconciliation precedes application of any new effects.
func (r *TaskTransitionPostgresRepository) WithTaskTransition(ctx context.Context, userID, orgID, workspaceID, id string, command workflowautomation.TransitionCommand, decide func(workflowautomation.TransitionSnapshot, workflowautomation.TransitionCommand) (workflowautomation.TransitionEffects, error)) (task.Task, error) {
	return withTaskChange(ctx, r.pool, r.queries, r.tasks, userID, orgID, workspaceID, id, command.Input.CommandID, func(snapshot task.TaskWorkflowSnapshot) (task.TaskWorkflowEffects, *task.TaskInput, error) {
		effects, err := decide(workflowautomation.TransitionSnapshot{Workflow: snapshot.Workflow, Task: snapshot.Task, ActiveLaunch: snapshot.ActiveLaunch, RecordedCommand: snapshot.RecordedCommand}, command)
		if err != nil {
			return task.TaskWorkflowEffects{}, nil, err
		}
		return task.TaskWorkflowEffects{Command: task.WorkflowCommand(effects.Input), StageID: effects.Plan.StageID, AssigneeAgentID: effects.Plan.AssigneeAgentID, Start: effects.Plan.Start, Completed: effects.CompleteStage, Moved: effects.Moved, StageVersion: effects.StageVersion}, nil, nil
	})
}

// persistWorkflowEffects is shared by user transitions and event processing.
// Callers already hold the task/configuration locks and have persisted movement.
func persistWorkflowEffects(ctx context.Context, q *sqlc.Queries, workspaceID, taskID, actorID string, version int64, moved bool, plan workflowautomation.StageTransitionPlan, in workflowautomation.TransitionRequest, cause, correlation string) error {
	if moved {
		if err := enqueueStageEntry(ctx, q, workspaceID, taskID, actorID, version, cause, correlation); err != nil {
			return err
		}
	}
	if plan.Start {
		return enqueueStageStart(ctx, q, workspaceID, taskID, actorID, version, in)
	}
	if plan.Completed {
		return q.SetStageWorkState(ctx, sqlc.SetStageWorkStateParams{WorkspaceID: workspaceID, ID: taskID, StageVersion: version, StageWorkState: string(task.WorkStateCompleted)})
	}
	return nil
}

type TaskTransitionPostgresRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
	tasks   *TaskPostgresRepository
}

func NewTaskTransitionPostgresRepository(database *PostgresDatabase) *TaskTransitionPostgresRepository {
	return &TaskTransitionPostgresRepository{pool: database.pool, queries: sqlc.New(database.pool), tasks: NewTaskPostgresRepository(database)}
}
