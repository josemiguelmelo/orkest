package postgres

import (
	"context"
	"errors"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	trigger "github.com/josemiguelmelo/orkest/internal/server/tasktriggerintegration"
)

func (r *TaskTriggerIntegrationPostgresRepository) NextProcessing(ctx context.Context) (trigger.ProcessingSnapshot, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return trigger.ProcessingSnapshot{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	row, err := q.NextTaskTriggerDelivery(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return trigger.ProcessingSnapshot{}, false, nil
	}
	if err != nil {
		return trigger.ProcessingSnapshot{}, false, err
	}
	snapshot, err := loadTriggerProcessing(ctx, q, row)
	return snapshot, err == nil, err
}

func loadTriggerProcessing(ctx context.Context, q *sqlc.Queries, row sqlc.TaskTriggerDelivery) (trigger.ProcessingSnapshot, error) {
	snapshot := trigger.ProcessingSnapshot{StageVersion: row.StageVersion}
	config, err := q.LockTaskTriggerIntegration(ctx, row.IntegrationID)
	if err != nil {
		return snapshot, err
	}
	snapshot.Integration, err = triggerIntegration(ctx, q, sqlc.GetTaskTriggerIntegrationRow(config))
	if err != nil {
		return snapshot, err
	}
	snapshot.Delivery, err = triggerDelivery(row)
	if err != nil {
		return snapshot, err
	}
	i := snapshot.Integration
	actor, err := q.GetWorkspace(ctx, sqlc.GetWorkspaceParams{OrganizationID: i.OrganizationID, ID: i.WorkspaceID, UserID: i.ConfiguredBy})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return snapshot, err
	}
	snapshot.ActorRole = actor.Role
	snapshot.Task, err = loadTaskChangeSnapshot(ctx, q, i.ConfiguredBy, i.OrganizationID, i.WorkspaceID, row.TaskID, row.ID)
	if errors.Is(err, problem.ErrNotFound) {
		return snapshot, nil
	}
	if err != nil {
		return snapshot, err
	}
	snapshot.TaskFound = true
	e := snapshot.Delivery.Event
	snapshot.LinkActive, err = q.TaskTriggerLinkActive(ctx, sqlc.TaskTriggerLinkActiveParams{IntegrationID: i.ID, TaskID: row.TaskID, RepositoryID: e.RepositoryID, ResourceID: e.ResourceID, ProviderName: i.Provider})
	return snapshot, err
}

func (r *TaskTriggerIntegrationPostgresRepository) CommitProcessing(ctx context.Context, expected trigger.ProcessingSnapshot, result trigger.ProcessingResult) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	row, err := q.LockTaskTriggerDelivery(ctx, expected.Delivery.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if row.Status != expected.Delivery.Status {
		return false, nil
	}
	current, err := loadTriggerProcessing(ctx, q, row)
	if err != nil {
		return false, err
	}
	if !reflect.DeepEqual(current, expected) {
		return false, nil
	}
	if result.Effects != nil {
		i := expected.Integration
		if err = persistTaskChange(ctx, q, i.ConfiguredBy, i.OrganizationID, i.WorkspaceID, row.TaskID, expected.Task, *result.Effects, nil); err != nil {
			return false, err
		}
	}
	if err = q.FinishTaskTriggerDelivery(ctx, sqlc.FinishTaskTriggerDeliveryParams{ID: row.ID, Status: result.Status}); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
