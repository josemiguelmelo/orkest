package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/task"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
)

type TaskPostgresRepository struct {
	queries *sqlc.Queries
	pool    *pgxpool.Pool
}

func NewTaskPostgresRepository(database *PostgresDatabase) *TaskPostgresRepository {
	return &TaskPostgresRepository{queries: sqlc.New(database.pool), pool: database.pool}
}

var _ task.TaskRepository = (*TaskPostgresRepository)(nil)

func (r *TaskPostgresRepository) Tasks(ctx context.Context, orgID, workspaceID string) ([]taskmodel.Task, error) {
	rows, err := r.queries.ListTasks(ctx, sqlc.ListTasksParams{OrganizationID: orgID, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	out := make([]taskmodel.Task, 0, len(rows))
	for _, row := range rows {
		t := taskFromRow(sqlc.GetTaskRow(row))
		out = append(out, t)
	}
	if len(out) == 0 {
		return out, nil
	}
	ids := make([]string, len(out))
	byID := make(map[string]int, len(out))
	for i := range out {
		ids[i] = out[i].ID
		byID[out[i].ID] = i
	}
	references, err := r.queries.ListWorkspaceTaskRepositories(ctx, sqlc.ListWorkspaceTaskRepositoriesParams{OrganizationID: orgID, WorkspaceID: workspaceID, TaskIds: ids})
	if err != nil {
		return nil, fmt.Errorf("list task repository references: %w", err)
	}
	for _, row := range references {
		index := byID[row.TaskID]
		out[index].Repositories = append(out[index].Repositories, taskmodel.RepositoryRef{ID: row.ID, ProviderName: row.ProviderName, Name: row.Name, URL: row.Url})
	}
	return out, nil
}

func (r *TaskPostgresRepository) Task(ctx context.Context, orgID, workspaceID, id string) (taskmodel.Task, error) {
	row, err := r.queries.GetTask(ctx, sqlc.GetTaskParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return taskmodel.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	t := taskFromRow(sqlc.GetTaskRow(row))
	if err = r.fillRepositories(ctx, &t); err != nil {
		return taskmodel.Task{}, err
	}
	return t, nil
}

func (r *TaskPostgresRepository) Comments(ctx context.Context, orgID, workspaceID, taskID string) ([]taskmodel.Comment, error) {
	rows, err := r.queries.ListTaskComments(ctx, sqlc.ListTaskCommentsParams{OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: taskID})
	if err != nil {
		return nil, err
	}
	out := make([]taskmodel.Comment, 0, len(rows))
	for _, row := range rows {
		out = append(out, taskmodel.Comment{ID: row.ID, WorkspaceID: row.WorkspaceID, TaskID: row.TaskID, AuthorUserID: row.AuthorUserID, Body: row.Body, CreatedAt: row.CreatedAt.Time})
	}
	return out, nil
}

func (r *TaskPostgresRepository) CreateComment(ctx context.Context, id, workspaceID, taskID, authorID, body string) (taskmodel.Comment, error) {
	// The organization is derived from the workspace, and the query binds both scopes.
	orgID, err := r.queries.GetWorkspaceOrganization(ctx, workspaceID)
	if err != nil {
		return taskmodel.Comment{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	row, err := r.queries.InsertTaskComment(ctx, sqlc.InsertTaskCommentParams{ID: id, OrganizationID: orgID, WorkspaceID: workspaceID, AuthorUserID: authorID, Body: body, ID_2: taskID})
	if err != nil {
		return taskmodel.Comment{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	return taskmodel.Comment{ID: row.ID, WorkspaceID: row.WorkspaceID, TaskID: row.TaskID, AuthorUserID: row.AuthorUserID, Body: row.Body, CreatedAt: row.CreatedAt.Time}, nil
}

func (r *TaskPostgresRepository) fillRepositories(ctx context.Context, t *taskmodel.Task) error {
	values, err := r.taskRepositories(ctx, t.WorkspaceID, t.ID)
	t.Repositories = values
	return err
}

func (r *TaskPostgresRepository) taskRepositories(ctx context.Context, workspaceID, taskID string) ([]taskmodel.RepositoryRef, error) {
	rows, err := r.queries.ListTaskRepositories(ctx, sqlc.ListTaskRepositoriesParams{WorkspaceID: workspaceID, TaskID: taskID})
	if err != nil {
		return nil, err
	}
	out := make([]taskmodel.RepositoryRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, taskmodel.RepositoryRef{ID: row.ID, ProviderName: row.ProviderName, Name: row.Name, URL: row.Url})
	}
	return out, nil
}

// All task queries return the same projection; conversions at call sites are
// checked by the compiler when SQL generation changes that projection.
func taskFromRow(row sqlc.GetTaskRow) taskmodel.Task {
	if row.Labels == nil {
		row.Labels = []string{}
	}
	return taskmodel.Task{ID: row.ID, WorkspaceID: row.WorkspaceID, WorkflowID: row.WorkflowID, Title: row.Title, Description: row.Description, SourceProvider: row.SourceProvider, SourceID: row.SourceID, SourceURL: row.SourceUrl, Status: row.Status, AssigneeAgentID: row.AssigneeAgentID, Labels: row.Labels, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, StageVersion: row.StageVersion, StageWorkState: taskmodel.WorkState(row.StageWorkState), AutomationPending: row.AutomationPending, Closed: row.Closed, StageName: row.StageName, StageDescription: row.StageDescription, Repositories: []taskmodel.RepositoryRef{}}
}
