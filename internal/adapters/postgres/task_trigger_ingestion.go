package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	trigger "github.com/josemiguelmelo/orkest/internal/server/tasktriggerintegration"
)

func (r *TaskTriggerIntegrationPostgresRepository) IngestionSnapshot(ctx context.Context, id string, event trigger.Event) (trigger.IngestionSnapshot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return trigger.IngestionSnapshot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return loadTriggerIngestion(ctx, sqlc.New(tx), id, event)
}

func loadTriggerIngestion(ctx context.Context, q *sqlc.Queries, id string, event trigger.Event) (trigger.IngestionSnapshot, error) {
	s := trigger.IngestionSnapshot{Event: event}
	config, err := q.LockTaskTriggerIntegration(ctx, id)
	if err != nil {
		return s, err
	}
	s.Integration, err = triggerIntegration(ctx, q, sqlc.GetTaskTriggerIntegrationRow(config))
	if err != nil {
		return s, err
	}
	if err = q.LockPullRequestWorkspace(ctx, s.Integration.WorkspaceID); err != nil {
		return s, err
	}
	if err = q.LockAssociationTasks(ctx, s.Integration.WorkspaceID); err != nil {
		return s, err
	}
	link, err := q.TaskTriggerIngestionLink(ctx, sqlc.TaskTriggerIngestionLinkParams{IntegrationID: id, RepositoryID: event.RepositoryID, ResourceID: event.ResourceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	s.LinkedTaskID, s.LinkCreatedAt, s.StageVersion, s.StageEnteredAt = link.TaskID, link.CreatedAt.Time, link.StageVersion, link.StageEnteredAt.Time
	return s, nil
}

func (r *TaskTriggerIntegrationPostgresRepository) CommitIngestion(ctx context.Context, expected trigger.IngestionSnapshot, delivery string, decision trigger.IngestionDecision) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	current, err := loadTriggerIngestion(ctx, q, expected.Integration.ID, expected.Event)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, expected) {
		return problem.ErrConflict
	}
	id, err := ids.NewUUID()
	if err != nil {
		return err
	}
	body, err := json.Marshal(expected.Event)
	if err != nil {
		return err
	}
	if err = q.InsertTaskTriggerDelivery(ctx, sqlc.InsertTaskTriggerDeliveryParams{ID: id, IntegrationID: expected.Integration.ID, DeliveryID: delivery, EventKey: expected.Event.Key, Event: body, TaskID: decision.TaskID, StageVersion: decision.StageVersion, Status: decision.Status}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
