package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/task"
	"github.com/josemiguelmelo/orkest/internal/workflow"
	"github.com/josemiguelmelo/orkest/internal/workflowautomation"
)

// withTaskChange shares locking and persistence between CRUD updates and stage commands.
// The supplied service callback decides the effects while the locks remain held.
func withTaskChange(ctx context.Context, pool *pgxpool.Pool, queries *sqlc.Queries, tasks *TaskPostgresRepository, userID, orgID, workspaceID, id, commandID string, decide func(task.TaskWorkflowSnapshot) (task.TaskWorkflowEffects, *task.TaskInput, error)) (task.Task, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return task.Task{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := queries.WithTx(tx)
	entry, err := q.TaskWorkflowState(ctx, sqlc.TaskWorkflowStateParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	data, err := q.LockWorkflow(ctx, sqlc.LockWorkflowParams{WorkspaceID: workspaceID, ID: entry.WorkflowID})
	if err != nil {
		return task.Task{}, err
	}
	d, err := workflowDefinition(ctx, q, data)
	if err != nil {
		return task.Task{}, err
	}
	state, err := q.LockWorkflowTask(ctx, sqlc.LockWorkflowTaskParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	if state.WorkflowID != entry.WorkflowID {
		return task.Task{}, modelerrors.ErrConflict
	}
	var previous *task.WorkflowCommand
	if commandID != "" {
		row, e := q.GetWorkflowCommand(ctx, sqlc.GetWorkflowCommandParams{WorkspaceID: workspaceID, TaskID: id, ID: commandID})
		if e == nil {
			previous = new(task.WorkflowCommand)
			if e = json.Unmarshal(row.Options, previous); e != nil {
				return task.Task{}, e
			}
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return task.Task{}, e
		}
	}
	row, err := q.GetTask(ctx, sqlc.GetTaskParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	active := false
	if previous == nil {
		if _, e := q.ActiveTaskLaunch(ctx, sqlc.ActiveTaskLaunchParams{WorkspaceID: workspaceID, TaskID: id}); e == nil {
			active = true
		} else if !errors.Is(e, pgx.ErrNoRows) {
			return task.Task{}, e
		}
	}
	decision, content, err := decide(task.TaskWorkflowSnapshot{Workflow: d, Task: taskFromRow(row), ActiveLaunch: active, RecordedCommand: previous != nil})
	if err != nil {
		return task.Task{}, err
	}
	in := decision.Command
	if previous != nil {
		if *previous != in {
			return task.Task{}, modelerrors.ErrConflict
		}
		// Release the transaction before reading through the pool again.
		if err = tx.Commit(ctx); err != nil {
			return task.Task{}, err
		}
		return tasks.Task(ctx, orgID, workspaceID, id)
	}
	plan := workflowautomation.StageTransitionPlan{StageID: decision.StageID, AssigneeAgentID: decision.AssigneeAgentID, Start: decision.Start, Completed: decision.Completed}

	if content != nil {
		if _, err = q.UpdateTask(ctx, sqlc.UpdateTaskParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id, Title: content.Title, Description: content.Description, Status: plan.StageID, Column7: plan.AssigneeAgentID, Labels: content.Labels}); err != nil {
			return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
		}
		if err = q.DeleteTaskRepositories(ctx, sqlc.DeleteTaskRepositoriesParams{WorkspaceID: workspaceID, TaskID: id}); err != nil {
			return task.Task{}, err
		}
		for _, repoID := range content.RepositoryIDs {
			if err = q.InsertTaskRepository(ctx, sqlc.InsertTaskRepositoryParams{WorkspaceID: workspaceID, TaskID: id, RepositoryID: repoID}); err != nil {
				return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
			}
		}
	} else if err = q.MoveWorkflowTask(ctx, sqlc.MoveWorkflowTaskParams{WorkspaceID: workspaceID, ID: id, StageVersion: state.StageVersion, Status: plan.StageID, Column5: plan.AssigneeAgentID}); err != nil {
		return task.Task{}, err
	}
	version := decision.StageVersion
	if decision.Completed {
		if err = q.EnqueueWorkflowEvent(ctx, sqlc.EnqueueWorkflowEventParams{ID: fmt.Sprintf("%s:%d:completed:%s", id, version, in.CommandID), WorkspaceID: workspaceID, TaskID: id, ActorID: userID, StageVersion: version, EventType: string(workflow.StageCompleted), RunID: state.WorkflowRunID, CausationID: in.CommandID, CorrelationID: in.CommandID}); err != nil {
			return task.Task{}, err
		}
	}
	if err = persistWorkflowEffects(ctx, q, workspaceID, id, userID, version, decision.Moved, plan, workflowautomation.TransitionRequest(in), in.CommandID, in.CommandID); err != nil {
		return task.Task{}, err
	}

	if in.CommandID != "" && (!plan.Start || in.Action == "move") {
		options, e := json.Marshal(in)
		if e != nil {
			return task.Task{}, e
		}
		if e = q.RecordWorkflowTransition(ctx, sqlc.RecordWorkflowTransitionParams{ID: in.CommandID, WorkspaceID: workspaceID, TaskID: id, StageVersion: version, ActorID: userID, Options: options}); e != nil {
			return task.Task{}, mapError(e, modelerrors.ErrConflict, modelerrors.ErrConflict)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return task.Task{}, mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
	}
	return tasks.Task(ctx, orgID, workspaceID, id)
}
