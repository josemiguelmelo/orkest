package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/workflow"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
)

type WorkflowDefinitionPostgresRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func NewWorkflowDefinitionPostgresRepository(database *PostgresDatabase) *WorkflowDefinitionPostgresRepository {
	return &WorkflowDefinitionPostgresRepository{pool: database.pool, queries: sqlc.New(database.pool)}
}

func workflowDefinition(ctx context.Context, q *sqlc.Queries, row sqlc.WorkspaceWorkflow) (workflowmodel.Definition, error) {
	stages, err := q.ListWorkflowStages(ctx, sqlc.ListWorkflowStagesParams{WorkspaceID: row.WorkspaceID, WorkflowID: row.ID})
	if err != nil {
		return workflowmodel.Definition{}, err
	}
	d := workflowmodel.Definition{ID: row.ID, Name: row.Name, Version: row.Version, ConfiguredBy: row.ConfiguredBy, InitialStageID: row.InitialStageID, ClosedStageID: row.ClosedStageID, Rules: []workflowmodel.AutomationRule{}, Stages: make([]workflowmodel.Stage, 0, len(stages))}
	for _, s := range stages {
		stage := workflowmodel.Stage{ID: s.ID, Name: s.Name, Description: s.Description, CustomPrompt: s.CustomPrompt, ResponsibleAgentID: s.ResponsibleAgentID.String}
		if err := json.Unmarshal(s.ArtifactInputs, &stage.ArtifactInputs); err != nil {
			return workflowmodel.Definition{}, err
		}
		if err := json.Unmarshal(s.ArtifactOutputs, &stage.ArtifactOutputs); err != nil {
			return workflowmodel.Definition{}, err
		}
		d.Stages = append(d.Stages, stage)
	}
	rules, err := q.ListWorkflowRules(ctx, sqlc.ListWorkflowRulesParams{WorkspaceID: row.WorkspaceID, WorkflowID: row.ID})
	if err != nil {
		return workflowmodel.Definition{}, err
	}
	for _, r := range rules {
		d.Rules = append(d.Rules, workflowmodel.AutomationRule{ID: r.ID, Enabled: r.Enabled, Event: workflowmodel.EventType(r.EventType), SourceStageID: r.SourceStageID, Action: workflowmodel.ActionType(r.ActionType), TargetStageID: r.TargetStageID.String})
	}
	return d, nil
}

func (r *WorkflowDefinitionPostgresRepository) Definitions(ctx context.Context, workspaceID string) ([]workflowmodel.Definition, error) {
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
	definitions := make([]workflowmodel.Definition, 0, len(rows))
	for _, row := range rows {
		d, e := workflowDefinition(ctx, q, row)
		if e != nil {
			return nil, e
		}
		definitions = append(definitions, d)
	}
	return definitions, tx.Commit(ctx)
}

func (r *WorkflowDefinitionPostgresRepository) Definition(ctx context.Context, workspaceID, workflowID string) (workflowmodel.Definition, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return workflowmodel.Definition{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	row, err := q.GetWorkflow(ctx, sqlc.GetWorkflowParams{WorkspaceID: workspaceID, ID: workflowID})
	if err != nil {
		return workflowmodel.Definition{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	d, err := workflowDefinition(ctx, q, row)
	if err != nil {
		return workflowmodel.Definition{}, err
	}
	return d, tx.Commit(ctx)
}

func (r *WorkflowDefinitionPostgresRepository) Create(ctx context.Context, workspaceID string, d workflowmodel.Definition, decide func(workflow.ConfigurationSnapshot, workflowmodel.Definition) (workflowmodel.Definition, error)) (workflowmodel.Definition, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return workflowmodel.Definition{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	snapshot, err := configurationSnapshot(ctx, q, workspaceID, d.ID, nil)
	if err != nil {
		return workflowmodel.Definition{}, err
	}
	d, err = decide(snapshot, d)
	if err != nil {
		return workflowmodel.Definition{}, err
	}

	if err = q.CreateWorkflow(ctx, sqlc.CreateWorkflowParams{WorkspaceID: workspaceID, ID: d.ID, Name: d.Name, Version: d.Version, ConfiguredBy: d.ConfiguredBy, InitialStageID: d.InitialStageID, ClosedStageID: d.ClosedStageID}); err != nil {
		return workflowmodel.Definition{}, mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
	}
	if err = r.saveStages(ctx, q, workspaceID, d); err != nil {
		return workflowmodel.Definition{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return workflowmodel.Definition{}, mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
	}
	return d, nil
}

func (r *WorkflowDefinitionPostgresRepository) Save(ctx context.Context, workspaceID string, d workflowmodel.Definition, decide func(workflow.ConfigurationSnapshot, workflowmodel.Definition) (workflowmodel.Definition, error)) (workflowmodel.Definition, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return workflowmodel.Definition{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	current, err := q.LockWorkflow(ctx, sqlc.LockWorkflowParams{WorkspaceID: workspaceID, ID: d.ID})
	if err != nil {
		return workflowmodel.Definition{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	definition, err := workflowDefinition(ctx, q, current)
	if err != nil {
		return workflowmodel.Definition{}, err
	}
	snapshot, err := configurationSnapshot(ctx, q, workspaceID, d.ID, &definition)
	if err != nil {
		return workflowmodel.Definition{}, err
	}
	d, err = decide(snapshot, d)
	if err != nil {
		return workflowmodel.Definition{}, err
	}

	if err = r.saveStages(ctx, q, workspaceID, d); err != nil {
		return workflowmodel.Definition{}, err
	}
	count, err := q.SaveWorkflow(ctx, sqlc.SaveWorkflowParams{WorkspaceID: workspaceID, ID: d.ID, Version: d.Version, Name: d.Name, ConfiguredBy: d.ConfiguredBy, InitialStageID: d.InitialStageID, ClosedStageID: d.ClosedStageID})
	if err != nil {
		return workflowmodel.Definition{}, err
	}
	if count != 1 {
		return workflowmodel.Definition{}, modelerrors.ErrConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return workflowmodel.Definition{}, mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
	}
	d.Version++
	return d, nil
}

func (r *WorkflowDefinitionPostgresRepository) saveStages(ctx context.Context, q *sqlc.Queries, workspaceID string, d workflowmodel.Definition) error {
	if err := q.DeleteWorkflowRules(ctx, sqlc.DeleteWorkflowRulesParams{WorkspaceID: workspaceID, WorkflowID: d.ID}); err != nil {
		return err
	}
	ids := make([]string, 0, len(d.Stages))
	for i, s := range d.Stages {
		ids = append(ids, s.ID)
		inputs, err := json.Marshal(s.ArtifactInputs)
		if err != nil {
			return err
		}
		outputs, err := json.Marshal(s.ArtifactOutputs)
		if err != nil {
			return err
		}
		if err := q.PutWorkflowStage(ctx, sqlc.PutWorkflowStageParams{WorkspaceID: workspaceID, WorkflowID: d.ID, ID: s.ID, AgentID: s.ResponsibleAgentID, Position: int32(i), Name: s.Name, Description: s.Description, CustomPrompt: s.CustomPrompt, ArtifactInputs: inputs, ArtifactOutputs: outputs}); err != nil {
			return err
		}
	}
	if err := q.RemoveWorkflowStages(ctx, sqlc.RemoveWorkflowStagesParams{WorkspaceID: workspaceID, WorkflowID: d.ID, Ids: ids}); err != nil {
		return mapError(err, modelerrors.ErrConflict, modelerrors.ErrConflict)
	}
	for _, r := range d.Rules {
		if err := q.InsertWorkflowRule(ctx, sqlc.InsertWorkflowRuleParams{WorkspaceID: workspaceID, WorkflowID: d.ID, ID: r.ID, Enabled: r.Enabled, EventType: string(r.Event), SourceStageID: r.SourceStageID, ActionType: string(r.Action), TargetStageID: r.TargetStageID}); err != nil {
			return err
		}
	}
	return nil
}

func (r *WorkflowDefinitionPostgresRepository) Delete(ctx context.Context, workspaceID, workflowID string, decide func(workflow.WorkflowDeletionSnapshot, string) error) error {
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
	referenced, err := q.WorkflowHasReferences(ctx, sqlc.WorkflowHasReferencesParams{WorkspaceID: workspaceID, WorkflowID: workflowID})
	if err != nil {
		return err
	}
	if err = decide(workflow.WorkflowDeletionSnapshot{WorkflowIDs: workflows, Referenced: referenced.Bool}, workflowID); err != nil {
		return err
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

func configurationSnapshot(ctx context.Context, q *sqlc.Queries, workspaceID, workflowID string, current *workflowmodel.Definition) (workflow.ConfigurationSnapshot, error) {
	agents, err := q.WorkflowConfigurationAgents(ctx, workspaceID)
	if err != nil {
		return workflow.ConfigurationSnapshot{}, err
	}
	stages, err := q.OccupiedWorkflowStages(ctx, sqlc.OccupiedWorkflowStagesParams{WorkspaceID: workspaceID, WorkflowID: workflowID})
	if err != nil {
		return workflow.ConfigurationSnapshot{}, err
	}
	return workflow.ConfigurationSnapshot{Current: current, AgentIDs: agents, OccupiedStageIDs: stages}, nil
}
