package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v93/github"
	codemodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
	"github.com/josemiguelmelo/orkest/internal/server/pullrequestlink"
	trigger "github.com/josemiguelmelo/orkest/internal/server/tasktriggerintegration"
)

type CredentialResolver interface {
	CodingCredential(context.Context, string, string) (codemodel.Credentials, error)
}

// Discovery fetches provider state using coding integration credentials. It does
// not grant task access, create links, or evaluate workflow rules.
type Discovery struct {
	Credentials CredentialResolver
	Client      *http.Client
}

func (d Discovery) Discover(ctx context.Context, workspaceID string, _ json.RawMessage, since time.Time, repositories []pullrequestlink.DiscoveryRepository) ([]trigger.Event, error) {
	var events []trigger.Event
	for _, repository := range repositories {
		discovered, err := d.discoverRepository(ctx, workspaceID, since, repository)
		if err != nil {
			return nil, err
		}
		events = append(events, discovered...)
	}
	return events, nil
}

func (d Discovery) discoverRepository(ctx context.Context, workspaceID string, since time.Time, repository pullrequestlink.DiscoveryRepository) ([]trigger.Event, error) {
	if d.Credentials == nil || repository.IntegrationID == "" {
		return nil, errors.New("discovery credentials unavailable")
	}
	credentials, err := d.Credentials.CodingCredential(ctx, workspaceID, repository.IntegrationID)
	if err != nil {
		return nil, errors.New("discovery credentials unavailable")
	}
	options := []github.ClientOptionsFunc{github.WithAuthToken(credentials.Token)}
	if d.Client != nil {
		options = append(options, github.WithHTTPClient(d.Client))
	}
	client, err := github.NewClient(options...)
	if err != nil {
		return nil, errors.New("discovery client unavailable")
	}
	repositoryID, _ := strconv.ParseInt(repository.ProviderRepositoryID, 10, 64)
	remote, _, err := client.Repositories.GetByID(ctx, repositoryID)
	if err != nil {
		return nil, errors.New("repository discovery failed")
	}
	owner, name := remote.GetOwner().GetLogin(), remote.GetName()
	cutoff := time.Now().Add(-24 * time.Hour)
	if since.After(cutoff) {
		cutoff = since
	}
	var events []trigger.Event
	page := 1
	for attempts := 0; attempts < 5; attempts++ {
		prs, response, err := client.PullRequests.List(ctx, owner, name, &github.PullRequestListOptions{State: "all", Sort: "updated", Direction: "desc", ListOptions: github.ListOptions{Page: page, PerPage: 100}})
		if err != nil {
			return nil, errors.New("pull request discovery failed")
		}
		reachedCutoff := false
		for _, pr := range prs {
			if pr.GetUpdatedAt().Before(cutoff) {
				reachedCutoff = true
				break
			}
			head := pr.GetHead()
			if !matchesObservedHead(head, repositoryID, repository.Candidates) {
				continue
			}
			resourceID := strconv.Itoa(pr.GetNumber())
			event := trigger.Event{Type: "pull_request.discovered", Key: discoveredEventKey(repository.ProviderRepositoryID, resourceID, head.GetSHA(), pr.GetUpdatedAt().Time), RepositoryID: repository.ProviderRepositoryID, ResourceID: resourceID, ReferenceURL: pr.GetHTMLURL(), OccurredAt: pr.GetUpdatedAt().Time, HeadRepositoryID: strconv.FormatInt(head.GetRepo().GetID(), 10), HeadBranch: head.GetRef(), HeadCommitID: head.GetSHA()}
			events = append(events, event)
			if !pr.GetMergedAt().IsZero() {
				merged := event
				merged.Type = PullRequestMerged
				merged.Key = mergedEventKey(repository.ProviderRepositoryID, resourceID)
				merged.OccurredAt = pr.GetMergedAt().Time
				events = append(events, merged)
			}
			reviews, err := discoverReviewEvents(ctx, client, remote, pr.GetNumber(), event, cutoff)
			if err != nil {
				return nil, err
			}
			events = append(events, reviews...)
		}
		if reachedCutoff || response.NextPage == 0 {
			return events, nil
		}
		page = response.NextPage
	}
	return nil, errors.New("pull request discovery limit exceeded")
}

func matchesObservedHead(head *github.PullRequestBranch, repositoryID int64, candidates []pullrequestlink.Candidate) bool {
	for _, candidate := range candidates {
		if head.GetRepo().GetID() == repositoryID && head.GetRef() == candidate.Branch && head.GetSHA() == candidate.CommitID {
			return true
		}
	}
	return false
}

func discoverReviewEvents(ctx context.Context, client *github.Client, remote *github.Repository, resourceNumber int, event trigger.Event, cutoff time.Time) ([]trigger.Event, error) {
	var events []trigger.Event
	reviewPage := 1
	for reviewAttempts := 0; reviewAttempts < 5; reviewAttempts++ {
		reviews, reviewResponse, err := client.PullRequests.ListReviews(ctx, remote.GetOwner().GetLogin(), remote.GetName(), resourceNumber, &github.ListOptions{Page: reviewPage, PerPage: 100})
		if err != nil {
			return nil, errors.New("review discovery failed")
		}
		for _, review := range reviews {
			if strings.EqualFold(review.GetState(), "PENDING") || strings.EqualFold(review.GetState(), "DISMISSED") || review.GetSubmittedAt().IsZero() || review.GetSubmittedAt().Before(cutoff) {
				continue
			}
			submitted := event
			submitted.Type = ReviewSubmitted
			submitted.Key = reviewEventKey(review.GetID())
			submitted.OccurredAt = review.GetSubmittedAt().Time
			events = append(events, submitted)
		}
		if reviewResponse.NextPage == 0 {
			break
		}
		if reviewAttempts == 4 {
			return nil, errors.New("review discovery limit exceeded")
		}
		reviewPage = reviewResponse.NextPage
	}
	return events, nil
}
