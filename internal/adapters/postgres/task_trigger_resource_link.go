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

func (r *TaskTriggerIntegrationPostgresRepository) ResourceLinkSnapshot(ctx context.Context, id string, link trigger.ResourceLink) (trigger.ResourceLinkSnapshot, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return trigger.ResourceLinkSnapshot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return loadResourceLink(ctx, sqlc.New(tx), id, link)
}

func loadResourceLink(ctx context.Context, q *sqlc.Queries, id string, link trigger.ResourceLink) (trigger.ResourceLinkSnapshot, error) {
	s := trigger.ResourceLinkSnapshot{Link: link}
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
	if err = q.LockAssociationTasks(ctx, i.WorkspaceID); err != nil {
		return s, err
	}
	states, err := q.PullRequestTaskStates(ctx, sqlc.PullRequestTaskStatesParams{WorkspaceID: i.WorkspaceID, ProviderName: i.Provider, ProviderRepositoryID: nullableText(link.RepositoryID)})
	if err != nil {
		return s, err
	}
	for _, state := range states {
		if state.ID == link.TaskID {
			s.RepositorySelected = state.RepositorySelected
			break
		}
	}
	owners, err := q.PullRequestResourceOwners(ctx, sqlc.PullRequestResourceOwnersParams{WorkspaceID: i.WorkspaceID, Provider: i.Provider, RepositoryID: link.RepositoryID, ResourceID: link.ResourceID})
	if err != nil {
		return s, err
	}
	for _, owner := range owners {
		s.OwnerTaskIDs = append(s.OwnerTaskIDs, owner.TaskID)
	}
	links, err := q.ListTaskTriggerResourceLinks(ctx, id)
	if err != nil {
		return s, err
	}
	for _, current := range links {
		if current.RepositoryID == link.RepositoryID && current.ResourceID == link.ResourceID {
			s.CurrentTaskID = current.TaskID
			s.CurrentLinkCreatedAt = current.CreatedAt.Time
			break
		}
	}
	s.ExistingTaskID, err = q.ExistingPullRequestTask(ctx, sqlc.ExistingPullRequestTaskParams{WorkspaceID: i.WorkspaceID, Provider: i.Provider, RepositoryID: link.RepositoryID, ResourceID: link.ResourceID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return s, err
	}
	s.Suppressed, err = q.TaskTriggerLinkSuppressed(ctx, sqlc.TaskTriggerLinkSuppressedParams{IntegrationID: id, RepositoryID: link.RepositoryID, ResourceID: link.ResourceID})
	return s, err
}

func (r *TaskTriggerIntegrationPostgresRepository) CommitResourceLink(ctx context.Context, expected trigger.ResourceLinkSnapshot, change trigger.ResourceLinkChange) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	current, err := loadResourceLink(ctx, q, expected.Integration.ID, expected.Link)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, expected) {
		return problem.ErrConflict
	}
	i, link := expected.Integration, expected.Link
	if change.Linked {
		count, err := q.PutTaskTriggerResourceLink(ctx, sqlc.PutTaskTriggerResourceLinkParams{IntegrationID: i.ID, WorkspaceID: i.WorkspaceID, TaskID: link.TaskID, RepositoryID: link.RepositoryID, ResourceID: link.ResourceID, Provider: i.Provider})
		if err != nil {
			return err
		}
		if count != 1 {
			return problem.ErrConflict
		}
	} else if err = q.DeleteTaskTriggerResourceLink(ctx, sqlc.DeleteTaskTriggerResourceLinkParams{IntegrationID: i.ID, RepositoryID: link.RepositoryID, ResourceID: link.ResourceID}); err != nil {
		return err
	}
	if change.Suppressed {
		err = q.SuppressTaskTriggerLink(ctx, sqlc.SuppressTaskTriggerLinkParams{IntegrationID: i.ID, RepositoryID: link.RepositoryID, ResourceID: link.ResourceID})
	} else {
		err = q.UnsuppressTaskTriggerLink(ctx, sqlc.UnsuppressTaskTriggerLinkParams{IntegrationID: i.ID, RepositoryID: link.RepositoryID, ResourceID: link.ResourceID})
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
