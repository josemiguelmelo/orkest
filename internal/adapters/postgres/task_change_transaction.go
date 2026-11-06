package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/task"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	"github.com/josemiguelmelo/orkest/internal/server/workflowautomation"
)

// withTaskChange shares locking and persistence between CRUD updates and stage commands.
// The supplied service callback decides the effects while the locks remain held.
func withTaskChange(ctx context.Context, pool *pgxpool.Pool, queries *sqlc.Queries, tasks *TaskPostgresRepository, userID, orgID, workspaceID, id, commandID string, decide func(task.TaskWorkflowSnapshot) (task.TaskWorkflowEffects, *taskmodel.TaskInput, error)) (taskmodel.Task, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return taskmodel.Task{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := queries.WithTx(tx)
	entry, err := q.TaskWorkflowState(ctx, sqlc.TaskWorkflowStateParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return taskmodel.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	data, err := q.LockWorkflow(ctx, sqlc.LockWorkflowParams{WorkspaceID: workspaceID, ID: entry.WorkflowID})
	if err != nil {
		return taskmodel.Task{}, err
	}
	d, err := workflowDefinition(ctx, q, data)
	if err != nil {
		return taskmodel.Task{}, err
	}
	state, err := q.LockWorkflowTask(ctx, sqlc.LockWorkflowTaskParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return taskmodel.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	if state.WorkflowID != entry.WorkflowID {
		return taskmodel.Task{}, modelerrors.ErrConflict
	}
	var previous *task.WorkflowCommand
	if commandID != "" {
		row, e := q.GetWorkflowCommand(ctx, sqlc.GetWorkflowCommandParams{WorkspaceID: workspaceID, TaskID: id, ID: commandID})
		if e == nil {
			previous = new(task.WorkflowCommand)
			if e = json.Unmarshal(row.Options, previous); e != nil {
				return taskmodel.Task{}, e
			}
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return taskmodel.Task{}, e
		}
	}
	row, err := q.GetTask(ctx, sqlc.GetTaskParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return taskmodel.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	active := false
	if previous == nil {
		if _, e := q.ActiveTaskLaunch(ctx, sqlc.ActiveTaskLaunchParams{WorkspaceID: workspaceID, TaskID: id}); e == nil {
			active = true
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return taskmodel.Task{}, e
		}
	}
	decision, content, err := decide(task.TaskWorkflowSnapshot{Workflow: d, Task: taskFromRow(row), ActiveLaunch: active, RecordedCommand: previous != nil, RecordedInput: previous, ActorID: userID, RunID: state.WorkflowRunID})
	if err != nil {
		return taskmodel.Task{}, err
	}
	if decision.Replay {
		if err = tx.Commit(ctx); err != nil {
			return taskmodel.Task{}, err
		}
		return tasks.Task(ctx, orgID, workspaceID, id)
	}

	plan := workflowautomation.StageTransitionPlan{StageID: decision.StageID, AssigneeAgentID: decision.AssigneeAgentID, Start: decision.Start, Completed: decision.Completed}

	if content != nil {
		if _, err = q.UpdateTask(ctx, sqlc.UpdateTaskParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id, Title: content.Title, Description: content.Description, Status: plan.StageID, Column7: plan.AssigneeAgentID, Labels: content.Labels}); err != nil {
			return taskmodel.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
		}
		if err = q.DeleteTaskRepositories(ctx, sqlc.DeleteTaskRepositoriesParams{WorkspaceID: workspaceID, TaskID: id}); err != nil {
			return taskmodel.Task{}, err
		}
		for _, repoID := range content.RepositoryIDs {
			if err = q.InsertTaskRepository(ctx, sqlc.InsertTaskRepositoryParams{WorkspaceID: workspaceID, TaskID: id, RepositoryID: repoID}); err != nil {
				return taskmodel.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
			}
		}
	} else if err = q.MoveWorkflowTask(ctx, sqlc.MoveWorkflowTaskParams{WorkspaceID: workspaceID, ID: id, StageVersion: state.StageVersion, Status: plan.StageID, Column5: plan.AssigneeAgentID}); err != nil {
		return taskmodel.Task{}, err
	}
	if err = persistStageEffects(ctx, q, workspaceID, id, userID, decision.Persistence); err != nil {
		return taskmodel.Task{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return taskmodel.Task{}, mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
	}
	return tasks.Task(ctx, orgID, workspaceID, id)
}
