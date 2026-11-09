package postgres

import (
	"context"
	"reflect"

	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	trigger "github.com/josemiguelmelo/orkest/internal/server/tasktriggerintegration"
)

func (r *TaskTriggerIntegrationPostgresRepository) UnlinkSnapshot(ctx context.Context, id string, link trigger.ResourceLink) (trigger.UnlinkSnapshot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return trigger.UnlinkSnapshot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return loadUnlink(ctx, sqlc.New(tx), id, link)
}

func loadUnlink(ctx context.Context, q *sqlc.Queries, id string, link trigger.ResourceLink) (trigger.UnlinkSnapshot, error) {
	s := trigger.UnlinkSnapshot{Link: link}
	row, err := q.LockTaskTriggerIntegration(ctx, id)
	if err != nil {
		return s, err
	}
	s.Integration, err = triggerIntegration(ctx, q, sqlc.GetTaskTriggerIntegrationRow(row))
	if err != nil {
		return s, err
	}
	i := s.Integration
	if err = q.LockPullRequestWorkspace(ctx, i.WorkspaceID); err != nil {
		return s, err
	}
	links, err := q.ListTaskTriggerResourceLinks(ctx, id)
	if err != nil {
		return s, err
	}
	for _, current := range links {
		if current.RepositoryID == link.RepositoryID && current.ResourceID == link.ResourceID {
			s.CurrentTaskID = current.TaskID
			break
		}
	}
	s.Suppressed, err = q.TaskTriggerLinkSuppressed(ctx, sqlc.TaskTriggerLinkSuppressedParams{IntegrationID: id, RepositoryID: link.RepositoryID, ResourceID: link.ResourceID})
	return s, err
}

func (r *TaskTriggerIntegrationPostgresRepository) CommitUnlink(ctx context.Context, expected trigger.UnlinkSnapshot) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	current, err := loadUnlink(ctx, q, expected.Integration.ID, expected.Link)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, expected) {
		return problem.ErrConflict
	}
	i, link := expected.Integration, expected.Link
	if err = q.DeleteTaskTriggerResourceLink(ctx, sqlc.DeleteTaskTriggerResourceLinkParams{IntegrationID: i.ID, RepositoryID: link.RepositoryID, ResourceID: link.ResourceID}); err != nil {
		return err
	}
	err = q.SuppressTaskTriggerLink(ctx, sqlc.SuppressTaskTriggerLinkParams{IntegrationID: i.ID, RepositoryID: link.RepositoryID, ResourceID: link.ResourceID})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
