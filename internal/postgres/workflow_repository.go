package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
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

func workflowDefinition(ctx context.Context, q *sqlc.Queries, row sqlc.WorkspaceWorkflow) (workflow.Definition, error) {
	stages, err := q.ListWorkflowStages(ctx, sqlc.ListWorkflowStagesParams{WorkspaceID: row.WorkspaceID, WorkflowID: row.ID})
	if err != nil {
		return workflow.Definition{}, err
	}
	d := workflow.Definition{ID: row.ID, Name: row.Name, Version: row.Version, ConfiguredBy: row.ConfiguredBy, Stages: make([]workflow.Stage, 0, len(stages))}
	for _, s := range stages {
		d.Stages = append(d.Stages, workflow.Stage{ID: s.ID, Name: s.Name, Description: s.Description, ResponsibleAgentID: s.ResponsibleAgentID.String, AutoStart: s.AutoStart, AutoAdvance: s.AutoAdvance})
	}
	return d, nil
}

func (r *WorkflowPostgresRepository) Definitions(ctx context.Context, workspaceID string) ([]workflow.Definition, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	rows, err := q.ListWorkflows(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	definitions := make([]workflow.Definition, 0, len(rows))
	for _, row := range rows {
		d, e := workflowDefinition(ctx, q, row)
		if e != nil {
			return nil, e
		}
		definitions = append(definitions, d)
	}
	return definitions, tx.Commit(ctx)
}

func (r *WorkflowPostgresRepository) Definition(ctx context.Context, workspaceID, workflowID string) (workflow.Definition, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return workflow.Definition{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	row, err := q.GetWorkflow(ctx, sqlc.GetWorkflowParams{WorkspaceID: workspaceID, ID: workflowID})
	if err != nil {
		return workflow.Definition{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	d, err := workflowDefinition(ctx, q, row)
	if err != nil {
		return workflow.Definition{}, err
	}
	return d, tx.Commit(ctx)
}

func (r *WorkflowPostgresRepository) Create(ctx context.Context, workspaceID string, d workflow.Definition) (workflow.Definition, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return workflow.Definition{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	if err = q.CreateWorkflow(ctx, sqlc.CreateWorkflowParams{WorkspaceID: workspaceID, ID: d.ID, Name: d.Name, Version: d.Version, ConfiguredBy: d.ConfiguredBy}); err != nil {
		return workflow.Definition{}, mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
	}
	if err = r.saveStages(ctx, q, workspaceID, d); err != nil {
		return workflow.Definition{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return workflow.Definition{}, mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
	}
	return d, nil
}

func (r *WorkflowPostgresRepository) Save(ctx context.Context, workspaceID string, d workflow.Definition) (workflow.Definition, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return workflow.Definition{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	current, err := q.LockWorkflow(ctx, sqlc.LockWorkflowParams{WorkspaceID: workspaceID, ID: d.ID})
	if err != nil {
		return workflow.Definition{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	if current.Version != d.Version {
		return workflow.Definition{}, modelerrors.ErrConflict
	}
	if err = r.saveStages(ctx, q, workspaceID, d); err != nil {
		return workflow.Definition{}, err
	}
	count, err := q.SaveWorkflow(ctx, sqlc.SaveWorkflowParams{WorkspaceID: workspaceID, ID: d.ID, Version: d.Version, Name: d.Name, ConfiguredBy: d.ConfiguredBy})
	if err != nil {
		return workflow.Definition{}, err
	}
	if count != 1 {
		return workflow.Definition{}, modelerrors.ErrConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return workflow.Definition{}, mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
	}
	d.Version++
	return d, nil
}

func (r *WorkflowPostgresRepository) saveStages(ctx context.Context, q *sqlc.Queries, workspaceID string, d workflow.Definition) error {
	ids := make([]string, 0, len(d.Stages))
	for i, s := range d.Stages {
		if s.ResponsibleAgentID != "" {
			exists, err := q.ValidateWorkflowAgent(ctx, sqlc.ValidateWorkflowAgentParams{WorkspaceID: workspaceID, ID: s.ResponsibleAgentID})
			if err != nil {
				return err
			}
			if !exists {
				return modelerrors.ErrInvalid
			}
		}
		ids = append(ids, s.ID)
		if err := q.PutWorkflowStage(ctx, sqlc.PutWorkflowStageParams{WorkspaceID: workspaceID, WorkflowID: d.ID, ID: s.ID, AgentID: s.ResponsibleAgentID, Position: int32(i), Name: s.Name, Description: s.Description, AutoStart: s.AutoStart, AutoAdvance: s.AutoAdvance}); err != nil {
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
