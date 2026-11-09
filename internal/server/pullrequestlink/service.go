// Package pullrequestlink associates provider PR identities with observed task branches.
// It never changes workflow state or starts execution.
package pullrequestlink

import (
	"context"
	"log/slog"
	"time"

	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
)

type Candidate struct {
	DefaultBranch      string
	EligibleForLinking bool
	Diagnostic         string
	TaskID             string
	RepositoryID       string
	HeadRepositoryID   string
	Branch             string
	CommitID           string
}

// CanAssociate interprets raw observation facts independently of persistence.
func (c Candidate) CanAssociate() bool {
	return c.EligibleForLinking && c.Diagnostic == "" && c.Branch != "" && c.Branch != c.DefaultBranch
}

type PullRequest struct {
	RepositoryID     string
	HeadRepositoryID string
	Branch           string
	CommitID         string
}

// Match requires exact repository, branch and commit evidence. More than one task is ambiguous.
func Match(pr PullRequest, candidates []Candidate) (string, string) {
	if pr.RepositoryID == "" || pr.HeadRepositoryID == "" || pr.Branch == "" || pr.CommitID == "" {
		return "", "missing_head_identity"
	}
	taskID := ""
	for _, candidate := range candidates {
		if !candidate.CanAssociate() {
			continue
		}
		if candidate.RepositoryID != pr.RepositoryID || candidate.HeadRepositoryID != pr.HeadRepositoryID || candidate.Branch != pr.Branch || candidate.CommitID != pr.CommitID {
			continue
		}
		if taskID != "" && taskID != candidate.TaskID {
			return "", "conflicting_branch_ownership"
		}
		taskID = candidate.TaskID
	}
	if taskID == "" {
		return "", "waiting_for_branch_evidence"
	}
	return taskID, ""
}

type PullRequestLinkRepository interface {
	EventResolution(context.Context, string, string) (ResolutionSnapshot, bool, error)
	NextResolution(context.Context) (ResolutionSnapshot, bool, error)
	CommitResolution(context.Context, ResolutionSnapshot, Resolution) (bool, error)
}

type PullRequestLinkService struct{ repository PullRequestLinkRepository }

func NewPullRequestLinkService(repository PullRequestLinkRepository) *PullRequestLinkService {
	return &PullRequestLinkService{repository: repository}
}

func (s *PullRequestLinkService) ResolveNext(ctx context.Context) (bool, error) {
	snapshot, found, err := s.repository.NextResolution(ctx)
	if err != nil || !found {
		return false, err
	}
	resolution := PlanResolution(snapshot, time.Now().UTC())
	return s.repository.CommitResolution(ctx, snapshot, resolution)
}

func (s *PullRequestLinkService) Run(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		more, err := s.ResolveNext(ctx)
		if err != nil {
			logger.Error("pull request association failed", "reason", "resolution_failed")
		}
		if more && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ResolveEvent associates the exact durable delivery received by the webhook.
// Missing evidence stays in the inbox for the background recovery worker.
func (s *PullRequestLinkService) ResolveEvent(ctx context.Context, integrationID, eventKey string) error {
	for attempt := 0; attempt < 3; attempt++ {
		snapshot, found, err := s.repository.EventResolution(ctx, integrationID, eventKey)
		if err != nil || !found {
			return err
		}
		applied, err := s.repository.CommitResolution(ctx, snapshot, PlanResolution(snapshot, time.Now().UTC()))
		if err != nil {
			return err
		}
		if applied {
			return nil
		}
	}
	return problem.ErrConflict
}
