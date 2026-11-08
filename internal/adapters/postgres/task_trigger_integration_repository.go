package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	"github.com/josemiguelmelo/orkest/internal/server/task"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
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

func (r *TaskTriggerIntegrationPostgresRepository) Save(ctx context.Context, i trigger.Integration, create bool) (trigger.Integration, error) {
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
	workflowIDs := make([]string, 0, len(i.Rules))
	for _, rule := range i.Rules {
		workflowIDs = append(workflowIDs, rule.WorkflowID)
	}
	sort.Strings(workflowIDs)
	for _, workflowID := range workflowIDs {
		row, e := q.LockWorkflow(ctx, sqlc.LockWorkflowParams{WorkspaceID: i.WorkspaceID, ID: workflowID})
		if e != nil {
			return i, mapError(e, problem.ErrInvalid, problem.ErrConflict)
		}
		definition, e := workflowDefinition(ctx, q, row)
		if e != nil {
			return i, e
		}
		for _, rule := range i.Rules {
			if rule.WorkflowID == workflowID {
				if e = trigger.ValidateRuleWorkflow(rule, definition); e != nil {
					return i, e
				}
			}
		}
	}
	for _, rule := range i.Rules {
		if err = q.InsertTaskTriggerRule(ctx, sqlc.InsertTaskTriggerRuleParams{IntegrationID: i.ID, WorkspaceID: i.WorkspaceID, ID: rule.ID, EventType: rule.Event, WorkflowID: rule.WorkflowID, SourceStageID: rule.SourceStageID, TargetStageID: rule.TargetStageID, Action: rule.Action, PromptText: rule.PromptText}); err != nil {
			return i, mapError(err, problem.ErrInvalid, problem.ErrConflict)
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

func (r *TaskTriggerIntegrationPostgresRepository) Link(ctx context.Context, i trigger.Integration, link trigger.ResourceLink) error {
	count, err := r.queries.PutTaskTriggerResourceLink(ctx, sqlc.PutTaskTriggerResourceLinkParams{IntegrationID: i.ID, WorkspaceID: i.WorkspaceID, TaskID: link.TaskID, RepositoryID: link.RepositoryID, ResourceID: link.ResourceID, Provider: i.Provider})
	if err != nil {
		return mapError(err, problem.ErrInvalid, problem.ErrConflict)
	}
	if count != 1 {
		return problem.ErrConflict
	}
	return nil
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

func (r *TaskTriggerIntegrationPostgresRepository) Unlink(ctx context.Context, id string, link trigger.ResourceLink) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	if _, err = q.LockTaskTriggerIntegration(ctx, id); err != nil {
		return err
	}
	if err = q.DeleteTaskTriggerResourceLink(ctx, sqlc.DeleteTaskTriggerResourceLinkParams{IntegrationID: id, RepositoryID: link.RepositoryID, ResourceID: link.ResourceID}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *TaskTriggerIntegrationPostgresRepository) Ingest(ctx context.Context, i trigger.Integration, delivery string, events []trigger.Event) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	current, err := q.LockTaskTriggerIntegration(ctx, i.ID)
	if err != nil {
		return err
	}
	// Configuration/secret rotation during verification must not accept old credentials.
	if current.Version != i.Version {
		return problem.ErrConflict
	}
	for _, event := range events {
		id, e := ids.NewUUID()
		if e != nil {
			return e
		}
		body, e := json.Marshal(event)
		if e != nil {
			return e
		}
		if err = q.InsertTaskTriggerDelivery(ctx, sqlc.InsertTaskTriggerDeliveryParams{ID: id, IntegrationID: i.ID, DeliveryID: delivery, EventKey: event.Key, Event: body, RepositoryID: event.RepositoryID, ResourceID: event.ResourceID, OccurredAt: pgtype.Timestamptz{Time: event.OccurredAt, Valid: true}}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func triggerDelivery(row sqlc.TaskTriggerDelivery) (trigger.Delivery, error) {
	d := trigger.Delivery{ID: row.ID, IntegrationID: row.IntegrationID, Status: row.Status, ReceivedAt: row.ReceivedAt.Time}
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

func (r *TaskTriggerIntegrationPostgresRepository) ProcessNext(ctx context.Context, decide func(context.Context, trigger.ProcessingSnapshot) (trigger.ProcessingResult, error)) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	row, err := q.NextTaskTriggerDelivery(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	config, err := q.LockTaskTriggerIntegration(ctx, row.IntegrationID)
	if err != nil {
		return false, err
	}
	i, err := triggerIntegration(ctx, q, sqlc.GetTaskTriggerIntegrationRow(config))
	if err != nil {
		return false, err
	}
	d, err := triggerDelivery(row)
	if err != nil {
		return false, err
	}
	actor, err := q.GetWorkspace(ctx, sqlc.GetWorkspaceParams{OrganizationID: i.OrganizationID, ID: i.WorkspaceID, UserID: i.ConfiguredBy})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	outcome := "ignored"
	err = applyTaskChange(ctx, q, i.ConfiguredBy, i.OrganizationID, i.WorkspaceID, row.TaskID, row.ID, func(t task.TaskWorkflowSnapshot) (task.TaskWorkflowEffects, *taskmodel.TaskInput, error) {
		linked, e := q.TaskTriggerLinkActive(ctx, sqlc.TaskTriggerLinkActiveParams{IntegrationID: i.ID, TaskID: row.TaskID, RepositoryID: d.Event.RepositoryID, ResourceID: d.Event.ResourceID, ProviderName: i.Provider})
		if e != nil {
			return task.TaskWorkflowEffects{}, nil, e
		}
		if !linked {
			return task.TaskWorkflowEffects{Replay: true}, nil, nil
		}
		result, e := decide(ctx, trigger.ProcessingSnapshot{ActorRole: actor.Role, Integration: i, Delivery: d, StageVersion: row.StageVersion, Task: t})
		if e != nil {
			return task.TaskWorkflowEffects{}, nil, e
		}
		outcome = result.Status
		if result.Effects == nil {
			return task.TaskWorkflowEffects{Replay: true}, nil, nil
		}
		return *result.Effects, nil, nil
	})
	if errors.Is(err, problem.ErrNotFound) {
		outcome = "ignored"
	} else if err != nil {
		return false, err
	}
	if err = q.FinishTaskTriggerDelivery(ctx, sqlc.FinishTaskTriggerDeliveryParams{ID: row.ID, Status: outcome}); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
