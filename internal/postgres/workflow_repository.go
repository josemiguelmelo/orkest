package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/workflow"
)

type WorkflowPostgresRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func NewWorkflowPostgresRepository(database *PostgresDatabase) *WorkflowPostgresRepository {
	return &WorkflowPostgresRepository{pool: database.pool, queries: sqlc.New(database.pool)}
}

func decodeWorkflow(data []byte) (workflow.Definition, error) {
	var d workflow.Definition
	err := json.Unmarshal(data, &d)
	return d, err
}

func (r *WorkflowPostgresRepository) Definitions(ctx context.Context, workspaceID string) ([]workflow.Definition, error) {
	rows, err := r.queries.ListWorkflows(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	definitions := make([]workflow.Definition, 0, len(rows))
	for _, row := range rows {
		definition, decodeErr := decodeWorkflow(row)
		if decodeErr != nil {
			return nil, decodeErr
		}
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

func (r *WorkflowPostgresRepository) Definition(ctx context.Context, workspaceID, workflowID string) (workflow.Definition, error) {
	data, err := r.queries.GetWorkflow(ctx, sqlc.GetWorkflowParams{WorkspaceID: workspaceID, ID: workflowID})
	if err != nil {
		return workflow.Definition{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	return decodeWorkflow(data)
}

func (r *WorkflowPostgresRepository) Create(ctx context.Context, workspaceID string, d workflow.Definition) (workflow.Definition, error) {
	return r.writeStages(ctx, workspaceID, d, true)
}

func (r *WorkflowPostgresRepository) Save(ctx context.Context, workspaceID string, d workflow.Definition) (workflow.Definition, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return workflow.Definition{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	data, err := q.LockWorkflow(ctx, sqlc.LockWorkflowParams{WorkspaceID: workspaceID, ID: d.ID})
	if err != nil {
		return workflow.Definition{}, err
	}
	current, err := decodeWorkflow(data)
	if err != nil {
		return workflow.Definition{}, err
	}
	if current.Version != d.Version {
		return workflow.Definition{}, modelerrors.ErrConflict
	}
	if current.Name != d.Name {
		current.Name = d.Name
	}
	if err = r.saveStages(ctx, q, workspaceID, d); err != nil {
		return workflow.Definition{}, err
	}
	d.Version++
	data, err = json.Marshal(d)
	if err != nil {
		return workflow.Definition{}, err
	}
	if _, err = q.SaveWorkflow(ctx, sqlc.SaveWorkflowParams{WorkspaceID: workspaceID, ID: d.ID, Version: current.Version, Name: d.Name, Definition: data}); err != nil {
		return workflow.Definition{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return workflow.Definition{}, mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
	}
	return d, nil
}

func (r *WorkflowPostgresRepository) writeStages(ctx context.Context, workspaceID string, d workflow.Definition, create bool) (workflow.Definition, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return workflow.Definition{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	data, err := json.Marshal(d)
	if err != nil {
		return workflow.Definition{}, err
	}
	if create {
		if err = q.CreateWorkflow(ctx, sqlc.CreateWorkflowParams{WorkspaceID: workspaceID, ID: d.ID, Name: d.Name, Version: d.Version, Definition: data}); err != nil {
			return workflow.Definition{}, mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
		}
	}
	if err = r.saveStages(ctx, q, workspaceID, d); err != nil {
		return workflow.Definition{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return workflow.Definition{}, mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
	}
	return d, nil
}

func (r *WorkflowPostgresRepository) saveStages(ctx context.Context, q *sqlc.Queries, workspaceID string, d workflow.Definition) error {
	ids := make([]string, 0, len(d.Stages))
	for _, s := range d.Stages {
		if s.ResponsibleAgentID != "" {
			exists, e := q.ValidateWorkflowAgent(ctx, sqlc.ValidateWorkflowAgentParams{WorkspaceID: workspaceID, ID: s.ResponsibleAgentID})
			if e != nil {
				return e
			}
			if !exists {
				return modelerrors.ErrInvalid
			}
		}
		ids = append(ids, s.ID)
		if err := q.PutWorkflowStage(ctx, sqlc.PutWorkflowStageParams{WorkspaceID: workspaceID, WorkflowID: d.ID, ID: s.ID, AgentID: s.ResponsibleAgentID}); err != nil {
			return err
		}
	}
	if err := q.RemoveWorkflowStages(ctx, sqlc.RemoveWorkflowStagesParams{WorkspaceID: workspaceID, WorkflowID: d.ID, Ids: ids}); err != nil {
		return mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
	}
	return nil
}

func (r *WorkflowPostgresRepository) Delete(ctx context.Context, workspaceID, workflowID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	workflows, err := q.LockWorkflows(ctx, workspaceID)
	if err != nil {
		return err
	}
	exists := false
	for _, id := range workflows {
		if id == workflowID {
			exists = true
			break
		}
	}
	if !exists {
		return modelerrors.ErrNotFound
	}
	if len(workflows) <= 1 {
		return modelerrors.ErrConflict
	}
	count, err := q.DeleteWorkflow(ctx, sqlc.DeleteWorkflowParams{WorkspaceID: workspaceID, ID: workflowID})
	if err != nil {
		return mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
	}
	if count == 0 {
		return modelerrors.ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
	}
	return nil
}
