package postgres

import (
	"context"
	"encoding/json"

	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	trigger "github.com/josemiguelmelo/orkest/internal/server/tasktriggerintegration"
)

func (r *TaskTriggerIntegrationPostgresRepository) EnqueueEvents(ctx context.Context, integrationID string, version int64, delivery string, events []trigger.Event, status string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	integration, err := q.LockTaskTriggerIntegration(ctx, integrationID)
	if err != nil {
		return err
	}
	if integration.Version != version {
		return problem.ErrConflict
	}
	for _, event := range events {
		id, err := ids.NewUUID()
		if err != nil {
			return err
		}
		body, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if err := q.InsertTaskTriggerDelivery(ctx, sqlc.InsertTaskTriggerDeliveryParams{ID: id, IntegrationID: integrationID, DeliveryID: delivery, EventKey: event.Key, Event: body, Status: status}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
