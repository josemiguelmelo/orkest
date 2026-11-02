package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/task"
)

type TaskUpdatePostgresRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
	tasks   *TaskPostgresRepository
}

func NewTaskUpdatePostgresRepository(database *PostgresDatabase) *TaskUpdatePostgresRepository {
	return &TaskUpdatePostgresRepository{pool: database.pool, queries: sqlc.New(database.pool), tasks: NewTaskPostgresRepository(database)}
}

func (r *TaskUpdatePostgresRepository) WithTaskUpdate(ctx context.Context, userID, orgID, workspaceID, id string, input task.TaskInput, decide func(task.TaskWorkflowSnapshot, task.TaskInput) (task.TaskUpdateEffects, error)) (task.Task, error) {
	return withTaskChange(ctx, r.pool, r.queries, r.tasks, userID, orgID, workspaceID, id, input.CommandID, func(snapshot task.TaskWorkflowSnapshot) (task.TaskWorkflowEffects, *task.TaskInput, error) {
		effects, err := decide(snapshot, input)
		if err != nil {
			return task.TaskWorkflowEffects{}, nil, err
		}
		return effects.Workflow, &effects.Content, nil
	})
}
