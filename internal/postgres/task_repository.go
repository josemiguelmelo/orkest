package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/task"
	"github.com/josemiguelmelo/orkest/internal/workflow"
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
		t := taskFrom(row.ID, row.WorkspaceID, row.WorkflowID, row.Title, row.Description, row.SourceProvider, row.SourceID, row.SourceUrl, row.Status, row.AssigneeAgentID, row.Labels, row.CreatedAt.Time, row.UpdatedAt.Time, row.StageVersion, row.StageWorkState, row.Closed, row.WorkflowSnapshot)
		if err := r.fillRepositories(ctx, &t); err != nil {
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
	t := taskFrom(row.ID, row.WorkspaceID, row.WorkflowID, row.Title, row.Description, row.SourceProvider, row.SourceID, row.SourceUrl, row.Status, row.AssigneeAgentID, row.Labels, row.CreatedAt.Time, row.UpdatedAt.Time, row.StageVersion, row.StageWorkState, row.Closed, row.WorkflowSnapshot)
	if err = r.fillRepositories(ctx, &t); err != nil {
		return task.Task{}, err
	}
	return t, nil
}

func (r *TaskPostgresRepository) CreateTask(ctx context.Context, id, workspaceID string, in task.TaskInput) (task.Task, error) {
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
	definition, e := decodeWorkflow(data)
	if e != nil {
		return task.Task{}, e
	}
	if in.Status == "" {
		in.Status = definition.StartStageID()
	}
	plan, e := workflow.PlanTransition(definition, in.Status, in.Status, in.AssigneeAgentID, workflow.Enter)
	if e != nil {
		return task.Task{}, e
	}
	in.AssigneeAgentID = plan.AssigneeAgentID
	row, err := q.InsertTask(ctx, sqlc.InsertTaskParams{ID: id, ID_2: workspaceID, WorkflowID: in.WorkflowID, Title: in.Title, Description: in.Description, Status: in.Status, Column7: in.AssigneeAgentID, Labels: in.Labels})
	if err != nil {
		return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	for _, repositoryID := range in.RepositoryIDs {
		if err = q.InsertTaskRepository(ctx, sqlc.InsertTaskRepositoryParams{WorkspaceID: workspaceID, TaskID: id, RepositoryID: repositoryID}); err != nil {
			return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
		}
	}
	if plan.Start && in.ActorID != "" {
		if err = enqueueStageStart(ctx, q, workspaceID, id, in.ActorID, 1, in); err != nil {
			return task.Task{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return task.Task{}, err
	}
	t := taskFrom(row.ID, row.WorkspaceID, row.WorkflowID, row.Title, row.Description, row.SourceProvider, row.SourceID, row.SourceUrl, row.Status, row.AssigneeAgentID, row.Labels, row.CreatedAt.Time, row.UpdatedAt.Time, row.StageVersion, row.StageWorkState, row.Closed, row.WorkflowSnapshot)
	err = r.fillRepositories(ctx, &t)
	return t, err
}

func (r *TaskPostgresRepository) UpdateTask(ctx context.Context, orgID, workspaceID, id string, in task.TaskInput) (task.Task, error) {
	return r.updateTask(ctx, orgID, workspaceID, id, in, "")
}

func (r *TaskPostgresRepository) UpdateTaskIfStatus(ctx context.Context, orgID, workspaceID, id, expectedStatus string, in task.TaskInput) (task.Task, error) {
	return r.updateTask(ctx, orgID, workspaceID, id, in, expectedStatus)
}

func (r *TaskPostgresRepository) updateTask(ctx context.Context, orgID, workspaceID, id string, in task.TaskInput, expectedStatus string) (task.Task, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return task.Task{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	var updated task.Task
	if expectedStatus == "" {
		var row sqlc.UpdateTaskRow
		row, err = q.UpdateTask(ctx, sqlc.UpdateTaskParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id, Title: in.Title, Description: in.Description, Status: in.Status, Column7: in.AssigneeAgentID, Labels: in.Labels})
		updated = taskFrom(row.ID, row.WorkspaceID, row.WorkflowID, row.Title, row.Description, row.SourceProvider, row.SourceID, row.SourceUrl, row.Status, row.AssigneeAgentID, row.Labels, row.CreatedAt.Time, row.UpdatedAt.Time, row.StageVersion, row.StageWorkState, row.Closed, row.WorkflowSnapshot)
	} else {
		var row sqlc.UpdateTaskIfStatusRow
		row, err = q.UpdateTaskIfStatus(ctx, sqlc.UpdateTaskIfStatusParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id, Status: expectedStatus, Title: in.Title, Description: in.Description, Status_2: in.Status, Column8: in.AssigneeAgentID, Labels: in.Labels})
		updated = taskFrom(row.ID, row.WorkspaceID, row.WorkflowID, row.Title, row.Description, row.SourceProvider, row.SourceID, row.SourceUrl, row.Status, row.AssigneeAgentID, row.Labels, row.CreatedAt.Time, row.UpdatedAt.Time, row.StageVersion, row.StageWorkState, row.Closed, row.WorkflowSnapshot)
	}
	if err != nil {
		if expectedStatus != "" {
			return task.Task{}, mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
		}
		return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	if err = q.DeleteTaskRepositories(ctx, sqlc.DeleteTaskRepositoriesParams{WorkspaceID: workspaceID, TaskID: id}); err != nil {
		return task.Task{}, err
	}
	for _, repositoryID := range in.RepositoryIDs {
		if err = q.InsertTaskRepository(ctx, sqlc.InsertTaskRepositoryParams{WorkspaceID: workspaceID, TaskID: id, RepositoryID: repositoryID}); err != nil {
			return task.Task{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return task.Task{}, err
	}
	updated.Repositories, err = r.taskRepositories(ctx, workspaceID, id)
	return updated, err
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

func (r *TaskPostgresRepository) fillRepositories(ctx context.Context, t *task.Task) error {
	d, e := decodeWorkflow(t.WorkflowSnapshot)
	if e != nil {
		return e
	}
	stage, _ := d.Stage(t.Status)
	t.StageName, t.StageDescription = stage.Name, stage.Description
	values, err := r.taskRepositories(ctx, t.WorkspaceID, t.ID)
	t.Repositories = values
	return err
}

func (r *TaskPostgresRepository) taskRepositories(ctx context.Context, workspaceID, taskID string) ([]task.RepositoryRef, error) {
	rows, err := r.queries.ListTaskRepositories(ctx, sqlc.ListTaskRepositoriesParams{WorkspaceID: workspaceID, TaskID: taskID})
	if err != nil {
		return nil, err
	}
	out := make([]task.RepositoryRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, task.RepositoryRef{ID: row.ID, ProviderName: row.ProviderName, Name: row.Name, URL: row.Url})
	}
	return out, nil
}

func taskFrom(id, workspaceID, workflowID, title, description, sourceProvider, sourceID, sourceURL, status, assignee string, labels []string, created, updated time.Time, stageVersion int64, workState string, closed bool, snapshot []byte) task.Task {
	if labels == nil {
		labels = []string{}
	}
	return task.Task{WorkflowSnapshot: snapshot, StageVersion: stageVersion, StageWorkState: task.WorkState(workState), WorkflowID: workflowID, Closed: closed, ID: id, WorkspaceID: workspaceID, Title: title, Description: description, SourceProvider: sourceProvider, SourceID: sourceID, SourceURL: sourceURL, Status: status, AssigneeAgentID: assignee, Labels: labels, Repositories: []task.RepositoryRef{}, CreatedAt: created, UpdatedAt: updated}
}
