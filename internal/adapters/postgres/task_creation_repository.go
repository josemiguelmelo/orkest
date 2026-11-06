package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/task"
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
	decision, e := decide(task.TaskCreationSnapshot{Workflow: definition, TaskID: id}, in)
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
	for _, event := range decision.Events {
		if err = persistStageEvent(ctx, q, workspaceID, id, in.ActorID, event); err != nil {
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
