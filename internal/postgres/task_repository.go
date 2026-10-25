package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/task"
)

type TaskPostgresRepository struct {
	queries *sqlc.Queries
	pool    *pgxpool.Pool
}

func NewTaskPostgresRepository(database *PostgresDatabase) *TaskPostgresRepository {
	return &TaskPostgresRepository{queries: sqlc.New(database.pool), pool: database.pool}
}

var _ task.Repository = (*TaskPostgresRepository)(nil)

func (r *TaskPostgresRepository) Tasks(ctx context.Context, orgID, workspaceID string) ([]task.Task, error) {
	rows, err := r.queries.ListTasks(ctx, sqlc.ListTasksParams{OrganizationID: orgID, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	out := make([]task.Task, 0, len(rows))
	for _, row := range rows {
		t := taskFrom(row.ID, row.WorkspaceID, row.Title, row.Description, row.SourceProvider, row.SourceID, row.SourceUrl, row.Status, row.AssigneeAgentID, row.Labels, row.CreatedAt.Time, row.UpdatedAt.Time)
		if err := r.fillComponents(ctx, &t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (r *TaskPostgresRepository) Task(ctx context.Context, orgID, workspaceID, id string) (task.Task, error) {
	row, err := r.queries.GetTask(ctx, sqlc.GetTaskParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	t := taskFrom(row.ID, row.WorkspaceID, row.Title, row.Description, row.SourceProvider, row.SourceID, row.SourceUrl, row.Status, row.AssigneeAgentID, row.Labels, row.CreatedAt.Time, row.UpdatedAt.Time)
	if err = r.fillComponents(ctx, &t); err != nil {
		return task.Task{}, err
	}
	return t, nil
}

func (r *TaskPostgresRepository) CreateTask(ctx context.Context, id, workspaceID string, in task.TaskInput) (task.Task, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return task.Task{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	row, err := q.InsertTask(ctx, sqlc.InsertTaskParams{ID: id, ID_2: workspaceID, Title: in.Title, Description: in.Description, Status: in.Status, Column6: in.AssigneeAgentID, Labels: in.Labels})
	if err != nil {
		return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	for _, componentID := range in.ComponentIDs {
		if err = q.InsertTaskComponent(ctx, sqlc.InsertTaskComponentParams{WorkspaceID: workspaceID, TaskID: id, ComponentID: componentID}); err != nil {
			return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return task.Task{}, err
	}
	t := taskFrom(row.ID, row.WorkspaceID, row.Title, row.Description, row.SourceProvider, row.SourceID, row.SourceUrl, row.Status, row.AssigneeAgentID, row.Labels, row.CreatedAt.Time, row.UpdatedAt.Time)
	t.Components, err = r.componentLinks(ctx, workspaceID, id)
	return t, err
}

func (r *TaskPostgresRepository) UpdateTask(ctx context.Context, orgID, workspaceID, id string, in task.TaskInput) (task.Task, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return task.Task{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	row, err := q.UpdateTask(ctx, sqlc.UpdateTaskParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id, Title: in.Title, Description: in.Description, Status: in.Status, Column7: in.AssigneeAgentID, Labels: in.Labels})
	if err != nil {
		return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	if err = q.DeleteTaskComponents(ctx, sqlc.DeleteTaskComponentsParams{WorkspaceID: workspaceID, TaskID: id}); err != nil {
		return task.Task{}, err
	}
	for _, componentID := range in.ComponentIDs {
		if err = q.InsertTaskComponent(ctx, sqlc.InsertTaskComponentParams{WorkspaceID: workspaceID, TaskID: id, ComponentID: componentID}); err != nil {
			return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return task.Task{}, err
	}
	t := taskFrom(row.ID, row.WorkspaceID, row.Title, row.Description, row.SourceProvider, row.SourceID, row.SourceUrl, row.Status, row.AssigneeAgentID, row.Labels, row.CreatedAt.Time, row.UpdatedAt.Time)
	t.Components, err = r.componentLinks(ctx, workspaceID, id)
	return t, err
}

func (r *TaskPostgresRepository) Comments(ctx context.Context, orgID, workspaceID, taskID string) ([]task.Comment, error) {
	rows, err := r.queries.ListTaskComments(ctx, sqlc.ListTaskCommentsParams{OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: taskID})
	if err != nil {
		return nil, err
	}
	out := make([]task.Comment, 0, len(rows))
	for _, row := range rows {
		out = append(out, task.Comment{ID: row.ID, WorkspaceID: row.WorkspaceID, TaskID: row.TaskID, AuthorUserID: row.AuthorUserID, Body: row.Body, CreatedAt: row.CreatedAt.Time})
	}
	return out, nil
}

func (r *TaskPostgresRepository) CreateComment(ctx context.Context, id, workspaceID, taskID, authorID, body string) (task.Comment, error) {
	// The organization is derived from the workspace, and the query binds both scopes.
	orgID, err := r.queries.GetWorkspaceOrganization(ctx, workspaceID)
	if err != nil {
		return task.Comment{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	row, err := r.queries.InsertTaskComment(ctx, sqlc.InsertTaskCommentParams{ID: id, OrganizationID: orgID, WorkspaceID: workspaceID, AuthorUserID: authorID, Body: body, ID_2: taskID})
	if err != nil {
		return task.Comment{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	return task.Comment{ID: row.ID, WorkspaceID: row.WorkspaceID, TaskID: row.TaskID, AuthorUserID: row.AuthorUserID, Body: row.Body, CreatedAt: row.CreatedAt.Time}, nil
}

func (r *TaskPostgresRepository) Components(ctx context.Context, orgID, workspaceID string) ([]task.Component, error) {
	rows, err := r.queries.ListComponents(ctx, sqlc.ListComponentsParams{OrganizationID: orgID, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	out := make([]task.Component, 0, len(rows))
	for _, row := range rows {
		out = append(out, componentFrom(row))
	}
	return out, nil
}

func (r *TaskPostgresRepository) CreateComponent(ctx context.Context, id, workspaceID string, in task.ComponentInput) (task.Component, error) {
	row, err := r.queries.InsertComponent(ctx, sqlc.InsertComponentParams{ID: id, ID_2: workspaceID, Identifier: in.Identifier, Name: in.Name, RepositoryID: in.RepositoryID, Description: in.Description})
	return componentFrom(row), mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (r *TaskPostgresRepository) UpdateComponent(ctx context.Context, orgID, workspaceID, id string, in task.ComponentInput) (task.Component, error) {
	row, err := r.queries.UpdateComponent(ctx, sqlc.UpdateComponentParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id, Identifier: in.Identifier, Name: in.Name, RepositoryID: in.RepositoryID, Description: in.Description})
	return componentFrom(row), mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (r *TaskPostgresRepository) fillComponents(ctx context.Context, t *task.Task) error {
	values, err := r.componentLinks(ctx, t.WorkspaceID, t.ID)
	t.Components = values
	return err
}

func (r *TaskPostgresRepository) componentLinks(ctx context.Context, workspaceID, taskID string) ([]task.Component, error) {
	rows, err := r.queries.ListTaskComponents(ctx, sqlc.ListTaskComponentsParams{WorkspaceID: workspaceID, TaskID: taskID})
	if err != nil {
		return nil, err
	}
	out := make([]task.Component, 0, len(rows))
	for _, row := range rows {
		out = append(out, componentFrom(row))
	}
	return out, nil
}

func taskFrom(id, workspaceID, title, description, sourceProvider, sourceID, sourceURL, status, assignee string, labels []string, created, updated time.Time) task.Task {
	if labels == nil {
		labels = []string{}
	}
	return task.Task{ID: id, WorkspaceID: workspaceID, Title: title, Description: description, SourceProvider: sourceProvider, SourceID: sourceID, SourceURL: sourceURL, Status: status, AssigneeAgentID: assignee, Labels: labels, Components: []task.Component{}, CreatedAt: created, UpdatedAt: updated}
}

func componentFrom(row sqlc.Component) task.Component {
	return task.Component{ID: row.ID, WorkspaceID: row.WorkspaceID, Identifier: row.Identifier, Name: row.Name, RepositoryID: row.RepositoryID, Description: row.Description, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}
}
