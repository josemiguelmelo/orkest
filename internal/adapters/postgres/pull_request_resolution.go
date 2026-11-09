package postgres

import (
	"context"
	"errors"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/pullrequestlink"
)

func (r *PullRequestLinkRepository) NextResolution(ctx context.Context) (pullrequestlink.ResolutionSnapshot, bool, error) {
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return pullrequestlink.ResolutionSnapshot{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	row, err := q.NextUnlinkedPullRequest(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return pullrequestlink.ResolutionSnapshot{}, false, nil
	}
	if err != nil {
		return pullrequestlink.ResolutionSnapshot{}, false, err
	}
	snapshot, err := loadPullRequestResolution(ctx, q, row)
	return snapshot, err == nil, err
}

func loadPullRequestResolution(ctx context.Context, q *sqlc.Queries, row sqlc.TaskTriggerDelivery) (pullrequestlink.ResolutionSnapshot, error) {
	s := pullrequestlink.ResolutionSnapshot{AvailableAt: row.ResolutionAfter.Time}
	integration, err := q.LockTaskTriggerIntegration(ctx, row.IntegrationID)
	if err != nil {
		return s, err
	}
	s.Integration, err = triggerIntegration(ctx, q, sqlc.GetTaskTriggerIntegrationRow(integration))
	if err != nil {
		return s, err
	}
	if err = q.LockPullRequestWorkspace(ctx, integration.WorkspaceID); err != nil {
		return s, err
	}
	if err = q.LockAssociationTasks(ctx, integration.WorkspaceID); err != nil {
		return s, err
	}
	if err = q.LockObservationLaunches(ctx, integration.WorkspaceID); err != nil {
		return s, err
	}
	s.Delivery, err = triggerDelivery(row)
	if err != nil {
		return s, err
	}
	e := s.Delivery.Event
	s.RepositoryRegistered, err = q.PullRequestRepositoryRegistered(ctx, sqlc.PullRequestRepositoryRegisteredParams{WorkspaceID: integration.WorkspaceID, ProviderName: integration.Provider, ProviderRepositoryID: pgtype.Text{String: e.RepositoryID, Valid: true}})
	if err != nil {
		return s, err
	}
	s.Suppressed, err = q.TaskTriggerLinkSuppressed(ctx, sqlc.TaskTriggerLinkSuppressedParams{IntegrationID: integration.ID, RepositoryID: e.RepositoryID, ResourceID: e.ResourceID})
	if err != nil {
		return s, err
	}
	s.ExistingTaskID, err = q.ExistingPullRequestTask(ctx, sqlc.ExistingPullRequestTaskParams{WorkspaceID: integration.WorkspaceID, Provider: integration.Provider, RepositoryID: e.RepositoryID, ResourceID: e.ResourceID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return s, err
	}
	candidates, err := q.PullRequestBranchCandidates(ctx, sqlc.PullRequestBranchCandidatesParams{WorkspaceID: integration.WorkspaceID, ProviderName: integration.Provider, ProviderRepositoryID: pgtype.Text{String: e.RepositoryID, Valid: true}})
	if err != nil {
		return s, err
	}
	for _, candidate := range candidates {
		s.Candidates = append(s.Candidates, pullRequestCandidate(candidate))
	}
	states, err := q.PullRequestTaskStates(ctx, sqlc.PullRequestTaskStatesParams{WorkspaceID: integration.WorkspaceID, ProviderName: integration.Provider, ProviderRepositoryID: pgtype.Text{String: e.RepositoryID, Valid: true}})
	if err != nil {
		return s, err
	}
	for _, state := range states {
		s.Tasks = append(s.Tasks, pullrequestlink.TaskState{TaskID: state.ID, RepositorySelected: state.RepositorySelected, StageVersion: state.StageVersion, StageEnteredAt: state.StageEnteredAt.Time})
	}
	return s, nil
}

func pullRequestCandidate(row sqlc.PullRequestBranchCandidatesRow) pullrequestlink.Candidate {
	return pullrequestlink.Candidate{DefaultBranch: row.DefaultBranch, EligibleForLinking: row.EligibleForLinking, Diagnostic: row.Diagnostic, TaskID: row.TaskID, RepositoryID: row.RepositoryID.String, HeadRepositoryID: row.RepositoryID.String, Branch: row.Branch, CommitID: row.CommitID}
}

func (r *PullRequestLinkRepository) CommitResolution(ctx context.Context, expected pullrequestlink.ResolutionSnapshot, result pullrequestlink.Resolution) (bool, error) {
	tx, err := r.database.pool.Begin(ctx)
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
	current, err := loadPullRequestResolution(ctx, q, row)
	if err != nil {
		return false, err
	}
	if !reflect.DeepEqual(current, expected) {
		return false, nil
	}
	i, e := expected.Integration, expected.Delivery.Event
	if result.Associate {
		count, err := q.AssociatePullRequest(ctx, sqlc.AssociatePullRequestParams{WorkspaceID: i.WorkspaceID, Provider: i.Provider, RepositoryID: e.RepositoryID, ResourceID: e.ResourceID, TaskID: result.TaskID, HeadRepositoryID: e.HeadRepositoryID, HeadBranch: e.HeadBranch})
		if err != nil {
			return false, err
		}
		if count != 1 {
			return false, problem.ErrConflict
		}
		count, err = q.PutTaskTriggerResourceLink(ctx, sqlc.PutTaskTriggerResourceLinkParams{IntegrationID: i.ID, WorkspaceID: i.WorkspaceID, RepositoryID: e.RepositoryID, ResourceID: e.ResourceID, TaskID: result.TaskID, Provider: i.Provider})
		if err != nil {
			return false, err
		}
		if count == 0 {
			active, err := q.TaskTriggerLinkActive(ctx, sqlc.TaskTriggerLinkActiveParams{IntegrationID: i.ID, TaskID: result.TaskID, RepositoryID: e.RepositoryID, ResourceID: e.ResourceID, ProviderName: i.Provider})
			if err != nil {
				return false, err
			}
			if !active {
				return false, problem.ErrConflict
			}
		}
	}
	count, err := q.SavePullRequestResolution(ctx, sqlc.SavePullRequestResolutionParams{ID: row.ID, TaskID: result.TaskID, StageVersion: result.StageVersion, Status: result.Status, Diagnostic: result.Diagnostic, RetryAt: pgtype.Timestamptz{Time: result.RetryAt, Valid: !result.RetryAt.IsZero()}, ExpectedStatus: expected.Delivery.Status})
	if err != nil {
		return false, err
	}
	if count != 1 {
		return false, problem.ErrConflict
	}
	return true, tx.Commit(ctx)
}

func (r *PullRequestLinkRepository) EventResolution(ctx context.Context, integrationID, eventKey string) (pullrequestlink.ResolutionSnapshot, bool, error) {
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return pullrequestlink.ResolutionSnapshot{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	row, err := q.PullRequestEventResolution(ctx, sqlc.PullRequestEventResolutionParams{IntegrationID: integrationID, EventKey: eventKey})
	if errors.Is(err, pgx.ErrNoRows) {
		return pullrequestlink.ResolutionSnapshot{}, false, nil
	}
	if err != nil {
		return pullrequestlink.ResolutionSnapshot{}, false, err
	}
	snapshot, err := loadPullRequestResolution(ctx, q, row)
	return snapshot, err == nil, err
}
