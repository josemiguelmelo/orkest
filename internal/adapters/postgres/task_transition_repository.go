package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/server/task"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	"github.com/josemiguelmelo/orkest/internal/server/workflowautomation"
)

// WithTaskTransition loads locked facts and persists the task policy's effects.
// Command reconciliation precedes application of any new effects.
func (r *TaskTransitionPostgresRepository) WithTaskTransition(ctx context.Context, userID, orgID, workspaceID, id string, command workflowautomation.TransitionCommand, decide func(workflowautomation.TransitionSnapshot, workflowautomation.TransitionCommand) (workflowautomation.TransitionEffects, error)) (taskmodel.Task, error) {
	return withTaskChange(ctx, r.pool, r.queries, r.tasks, userID, orgID, workspaceID, id, command.Input.CommandID, func(snapshot task.TaskWorkflowSnapshot) (task.TaskWorkflowEffects, *taskmodel.TaskInput, error) {
		effects, err := decide(workflowautomation.TransitionSnapshot{Workflow: snapshot.Workflow, Task: snapshot.Task, ActiveLaunch: snapshot.ActiveLaunch, RecordedCommand: snapshot.RecordedCommand, RecordedInput: snapshot.RecordedInput, ActorID: snapshot.ActorID, RunID: snapshot.RunID, CandidateCommandID: snapshot.CandidateCommandID}, command)
		if err != nil {
			return task.TaskWorkflowEffects{}, nil, err
		}
		return task.TaskWorkflowEffects{Command: task.WorkflowCommand(effects.Input), Persistence: effects.Persistence, Replay: effects.Replay, StageID: effects.Plan.StageID, AssigneeAgentID: effects.Plan.AssigneeAgentID, Start: effects.Plan.Start, Completed: effects.CompleteStage, Moved: effects.Moved, StageVersion: effects.StageVersion}, nil, nil
	})
}

type TaskTransitionPostgresRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
	tasks   *TaskPostgresRepository
}

func NewTaskTransitionPostgresRepository(database *PostgresDatabase) *TaskTransitionPostgresRepository {
	return &TaskTransitionPostgresRepository{pool: database.pool, queries: sqlc.New(database.pool), tasks: NewTaskPostgresRepository(database)}
}
