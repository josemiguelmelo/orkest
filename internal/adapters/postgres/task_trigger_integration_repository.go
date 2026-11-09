package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	trigger "github.com/josemiguelmelo/orkest/internal/server/tasktriggerintegration"
)

type TaskTriggerIntegrationPostgresRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func NewTaskTriggerIntegrationPostgresRepository(database *PostgresDatabase) *TaskTriggerIntegrationPostgresRepository {
	return &TaskTriggerIntegrationPostgresRepository{pool: database.pool, queries: sqlc.New(database.pool)}
}

var _ trigger.TaskTriggerIntegrationRepository = (*TaskTriggerIntegrationPostgresRepository)(nil)

func triggerIntegration(ctx context.Context, q *sqlc.Queries, row sqlc.GetTaskTriggerIntegrationRow) (trigger.Integration, error) {
	i := trigger.Integration{ID: row.ID, WorkspaceID: row.WorkspaceID, OrganizationID: row.OrganizationID, Name: row.Name, Provider: row.Provider, Config: row.Config, Enabled: row.Enabled, Version: row.Version, ConfiguredBy: row.ConfiguredBy, EncryptedSecret: row.EncryptedSecret, Rules: []trigger.Rule{}}
	diagnostic, err := q.PullRequestDiscoveryDiagnostic(ctx, row.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return i, err
	}
	i.DiscoveryDiagnostic = diagnostic
	rules, err := q.ListTaskTriggerRules(ctx, row.ID)
	if err != nil {
		return i, err
	}
	for _, r := range rules {
		i.Rules = append(i.Rules, trigger.Rule{ID: r.ID, Event: r.EventType, WorkflowID: r.WorkflowID, SourceStageID: r.SourceStageID, TargetStageID: r.TargetStageID, Action: r.Action, PromptText: r.PromptText})
	}
	return i, nil
}

func (r *TaskTriggerIntegrationPostgresRepository) List(ctx context.Context, workspace string) ([]trigger.Integration, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	rows, err := q.ListTaskTriggerIntegrations(ctx, workspace)
	if err != nil {
		return nil, err
	}
	result := make([]trigger.Integration, 0, len(rows))
	for _, row := range rows {
		i, e := triggerIntegration(ctx, q, sqlc.GetTaskTriggerIntegrationRow(row))
		if e != nil {
			return nil, e
		}
		result = append(result, i)
	}
	return result, tx.Commit(ctx)
}

func (r *TaskTriggerIntegrationPostgresRepository) Get(ctx context.Context, id string) (trigger.Integration, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return trigger.Integration{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	row, err := q.GetTaskTriggerIntegration(ctx, id)
	if err != nil {
		return trigger.Integration{}, mapError(err, problem.ErrNotFound, problem.ErrConflict)
	}
	i, err := triggerIntegration(ctx, q, row)
	if err != nil {
		return i, err
	}
	return i, tx.Commit(ctx)
}

func (r *TaskTriggerIntegrationPostgresRepository) Save(ctx context.Context, plan trigger.ConfigurationPlan) (trigger.Integration, error) {
	i, create := plan.Integration, plan.Create
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return i, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	if create {
		err = q.CreateTaskTriggerIntegration(ctx, sqlc.CreateTaskTriggerIntegrationParams{ID: i.ID, WorkspaceID: i.WorkspaceID, Name: i.Name, Provider: i.Provider, Config: i.Config, Enabled: i.Enabled, ConfiguredBy: i.ConfiguredBy, EncryptedSecret: i.EncryptedSecret})
	} else {
		var count int64
		count, err = q.UpdateTaskTriggerIntegration(ctx, sqlc.UpdateTaskTriggerIntegrationParams{ID: i.ID, Version: i.Version, Name: i.Name, Config: i.Config, Enabled: i.Enabled, ConfiguredBy: i.ConfiguredBy, EncryptedSecret: i.EncryptedSecret})
		if err == nil && count != 1 {
			return i, problem.ErrConflict
		}
	}
	if err != nil {
		return i, mapError(err, problem.ErrConflict, problem.ErrConflict)
	}
	if err = q.DeleteTaskTriggerRules(ctx, i.ID); err != nil {
		return i, err
	}
	for _, expected := range plan.Workflows {
		row, err := q.LockWorkflow(ctx, sqlc.LockWorkflowParams{WorkspaceID: i.WorkspaceID, ID: expected.ID})
		if err != nil {
			return i, mapError(err, problem.ErrInvalid, problem.ErrConflict)
		}
		current, err := workflowDefinition(ctx, q, row)
		if err != nil {
			return i, err
		}
		if !reflect.DeepEqual(current, expected) {
			return i, problem.ErrConflict
		}
	}
	for _, rule := range i.Rules {
		if err = q.InsertTaskTriggerRule(ctx, sqlc.InsertTaskTriggerRuleParams{IntegrationID: i.ID, WorkspaceID: i.WorkspaceID, ID: rule.ID, EventType: rule.Event, WorkflowID: rule.WorkflowID, SourceStageID: rule.SourceStageID, TargetStageID: rule.TargetStageID, Action: rule.Action, PromptText: rule.PromptText}); err != nil {
			return i, mapError(err, problem.ErrInvalid, problem.ErrConflict)
		}
	}
	if plan.ResetDiscovery {
		if err = q.ResetPullRequestDiscovery(ctx, i.ID); err != nil {
			return i, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return i, mapError(err, problem.ErrInvalid, problem.ErrConflict)
	}
	if create {
		i.Version = 1
	} else {
		i.Version++
	}
	return i, nil
}

func (r *TaskTriggerIntegrationPostgresRepository) Delete(ctx context.Context, workspace, id string) error {
	_, err := r.queries.DeleteTaskTriggerIntegration(ctx, sqlc.DeleteTaskTriggerIntegrationParams{WorkspaceID: workspace, ID: id})
	return err
}

func (r *TaskTriggerIntegrationPostgresRepository) Links(ctx context.Context, id string) ([]trigger.ResourceLink, error) {
	rows, err := r.queries.ListTaskTriggerResourceLinks(ctx, id)
	if err != nil {
		return nil, err
	}
	result := make([]trigger.ResourceLink, 0, len(rows))
	for _, row := range rows {
		result = append(result, trigger.ResourceLink{TaskID: row.TaskID, RepositoryID: row.RepositoryID, ResourceID: row.ResourceID})
	}
	return result, nil
}

func triggerDelivery(row sqlc.TaskTriggerDelivery) (trigger.Delivery, error) {
	d := trigger.Delivery{ID: row.ID, IntegrationID: row.IntegrationID, Status: row.Status, Diagnostic: row.Diagnostic, ReceivedAt: row.ReceivedAt.Time}
	err := json.Unmarshal(row.Event, &d.Event)
	return d, err
}

func (r *TaskTriggerIntegrationPostgresRepository) Deliveries(ctx context.Context, id string) ([]trigger.Delivery, error) {
	rows, err := r.queries.ListTaskTriggerDeliveries(ctx, id)
	if err != nil {
		return nil, err
	}
	result := make([]trigger.Delivery, 0, len(rows))
	for _, row := range rows {
		d, e := triggerDelivery(row)
		if e != nil {
			return nil, e
		}
		result = append(result, d)
	}
	return result, nil
}
