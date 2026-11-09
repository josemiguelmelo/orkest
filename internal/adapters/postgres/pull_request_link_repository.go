package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/server/pullrequestlink"
)

type PullRequestLinkRepository struct{ database *PostgresDatabase }

func NewPullRequestLinkRepository(database *PostgresDatabase) *PullRequestLinkRepository {
	return &PullRequestLinkRepository{database: database}
}

func (r *PullRequestLinkRepository) ClaimDiscovery(ctx context.Context) (pullrequestlink.DiscoveryTarget, bool, error) {
	target := pullrequestlink.DiscoveryTarget{}
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return target, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	if err = q.InitializePullRequestDiscovery(ctx); err != nil {
		return target, false, err
	}
	claim, err := q.ClaimPullRequestDiscovery(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return target, false, nil
	}
	if err != nil {
		return target, false, err
	}
	integration, err := q.GetTaskTriggerIntegration(ctx, claim.IntegrationID)
	if err != nil {
		return target, false, err
	}
	target.Since = claim.ActivatedAt.Time
	target.Integration, err = triggerIntegration(ctx, q, integration)
	if err != nil {
		return target, false, err
	}
	repositories, err := q.PullRequestDiscoveryRepositories(ctx, sqlc.PullRequestDiscoveryRepositoriesParams{WorkspaceID: integration.WorkspaceID, ProviderName: integration.Provider})
	if err != nil {
		return target, false, err
	}
	for _, repository := range repositories {
		candidates, err := q.PullRequestBranchCandidates(ctx, sqlc.PullRequestBranchCandidatesParams{WorkspaceID: integration.WorkspaceID, ProviderName: integration.Provider, ProviderRepositoryID: repository.ProviderRepositoryID})
		if err != nil {
			return target, false, err
		}
		value := pullrequestlink.DiscoveryRepository{ID: repository.ID, ProviderRepositoryID: repository.ProviderRepositoryID.String, IntegrationID: repository.IntegrationID.String, URL: repository.Url}
		for _, candidate := range candidates {
			value.Candidates = append(value.Candidates, pullRequestCandidate(candidate))
		}
		target.Repositories = append(target.Repositories, value)
	}
	return target, true, tx.Commit(ctx)
}

func (r *PullRequestLinkRepository) DiscoveryFinished(ctx context.Context, id, diagnostic string) error {
	return sqlc.New(r.database.pool).FinishPullRequestDiscovery(ctx, sqlc.FinishPullRequestDiscoveryParams{IntegrationID: id, Diagnostic: diagnostic})
}
