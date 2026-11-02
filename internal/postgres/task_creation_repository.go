package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/task"
)

type TaskCreationPostgresRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
	tasks   *TaskPostgresRepository
}

func NewTaskCreationPostgresRepository(database *PostgresDatabase) *TaskCreationPostgresRepository {
	return &TaskCreationPostgresRepository{pool: database.pool, queries: sqlc.New(database.pool), tasks: NewTaskPostgresRepository(database)}
}

func (r *TaskCreationPostgresRepository) WithTaskCreation(ctx context.Context, id, workspaceID string, in task.TaskInput, decide func(task.TaskCreationSnapshot, task.TaskInput) (task.TaskCreationEffects, error)) (task.Task, error) {
	if in.Labels == nil {
		in.Labels = []string{}
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return task.Task{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	data, e := q.LockWorkflow(ctx, sqlc.LockWorkflowParams{WorkspaceID: workspaceID, ID: in.WorkflowID})
	if e != nil {
		return task.Task{}, e
	}
	definition, e := workflowDefinition(ctx, q, data)
	if e != nil {
		return task.Task{}, e
	}
	decision, e := decide(task.TaskCreationSnapshot{Workflow: definition}, in)
	if e != nil {
		return task.Task{}, e
	}
	in = decision.Input

	row, err := q.InsertTask(ctx, sqlc.InsertTaskParams{ID: id, ID_2: workspaceID, WorkflowID: in.WorkflowID, Title: in.Title, Description: in.Description, Status: in.Status, Column7: in.AssigneeAgentID, Labels: in.Labels})
	if err != nil {
		return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	for _, repositoryID := range in.RepositoryIDs {
		if err = q.InsertTaskRepository(ctx, sqlc.InsertTaskRepositoryParams{WorkspaceID: workspaceID, TaskID: id, RepositoryID: repositoryID}); err != nil {
			return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
		}
	}
	if decision.EmitStageEntry {
		if err = enqueueStageEntry(ctx, q, workspaceID, id, in.ActorID, 1, in.CommandID); err != nil {
			return task.Task{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return task.Task{}, err
	}
	t := taskFromRow(sqlc.GetTaskRow(row))
	t.AutomationPending = decision.EmitStageEntry
	err = r.tasks.fillRepositories(ctx, &t)
	return t, err
}
