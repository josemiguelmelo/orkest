package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/taskintegration"
	trigger "github.com/josemiguelmelo/orkest/internal/server/tasktriggerintegration"
)

const (
	ReviewSubmitted   = "pull_request.review_submitted"
	PullRequestMerged = "pull_request.merged"
)

var repositoryName = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

type GitHubProvider struct{}

type configuration struct {
	RepositoryIDs []string `json:"repositoryIds" jsonschema:"title=Repository IDs,description=GitHub numeric repository IDs,minItems=1,uniqueItems=true"`
}

func NewGitHubProvider() *GitHubProvider { return &GitHubProvider{} }

func (*GitHubProvider) Descriptor() trigger.ProviderDescriptor {
	return trigger.ProviderDescriptor{ID: "github", Name: "GitHub", ConfigSchema: taskintegration.GenerateProviderConfigSchema(configuration{}, "GitHub webhook"), Events: []string{ReviewSubmitted, PullRequestMerged}}
}

func decodeConfiguration(raw json.RawMessage) (configuration, error) {
	c, err := taskintegration.DecodeProviderConfig[configuration](raw)
	if err != nil || len(c.RepositoryIDs) == 0 {
		return configuration{}, problem.ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range c.RepositoryIDs {
		if !positiveID(id) || seen[id] {
			return configuration{}, problem.ErrInvalid
		}
		seen[id] = true
	}
	return c, nil
}

func (*GitHubProvider) ValidateConfig(raw json.RawMessage) error {
	_, err := decodeConfiguration(raw)
	return err
}

func (*GitHubProvider) ValidateResource(raw json.RawMessage, link trigger.ResourceLink) error {
	c, err := decodeConfiguration(raw)
	if err != nil || !slices.Contains(c.RepositoryIDs, link.RepositoryID) || !positiveID(link.ResourceID) {
		return problem.ErrInvalid
	}
	return nil
}

func positiveID(value string) bool {
	n, err := strconv.ParseInt(value, 10, 64)
	return err == nil && n > 0 && strconv.FormatInt(n, 10) == value
}

func (*GitHubProvider) VerifyAndDecode(ctx context.Context, request trigger.WebhookRequest, raw json.RawMessage, secret string) (string, []trigger.Event, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	signature, err := hex.DecodeString(strings.TrimPrefix(request.Headers["X-Hub-Signature-256"], "sha256="))
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(request.Body)
	if secret == "" || err != nil || !strings.HasPrefix(request.Headers["X-Hub-Signature-256"], "sha256=") || !hmac.Equal(signature, mac.Sum(nil)) {
		return "", nil, problem.ErrForbidden
	}
	deliveryID := request.Headers["X-Github-Delivery"]
	if deliveryID == "" || len(deliveryID) > 200 {
		return "", nil, problem.ErrInvalid
	}
	var payload struct {
		Action     string `json:"action"`
		Repository struct {
			ID       int64  `json:"id"`
			FullName string `json:"full_name"`
		} `json:"repository"`
		PullRequest struct {
			UpdatedAt time.Time `json:"updated_at"`
			Head      struct {
				Ref  string `json:"ref"`
				SHA  string `json:"sha"`
				Repo struct {
					ID int64 `json:"id"`
				} `json:"repo"`
			} `json:"head"`
			Number   int64     `json:"number"`
			Merged   bool      `json:"merged"`
			MergedAt time.Time `json:"merged_at"`
		} `json:"pull_request"`
		Review struct {
			ID          int64     `json:"id"`
			SubmittedAt time.Time `json:"submitted_at"`
		} `json:"review"`
	}
	if json.Unmarshal(request.Body, &payload) != nil {
		return "", nil, problem.ErrInvalid
	}
	config, err := decodeConfiguration(raw)
	if err != nil {
		return "", nil, problem.ErrInvalid
	}
	if request.Headers["X-Github-Event"] == "ping" {
		return deliveryID, nil, nil
	}
	repositoryID := strconv.FormatInt(payload.Repository.ID, 10)
	if !slices.Contains(config.RepositoryIDs, repositoryID) {
		return "", nil, problem.ErrForbidden
	}
	event := trigger.Event{HeadRepositoryID: strconv.FormatInt(payload.PullRequest.Head.Repo.ID, 10), HeadBranch: payload.PullRequest.Head.Ref, HeadCommitID: payload.PullRequest.Head.SHA, RepositoryID: repositoryID, ResourceID: strconv.FormatInt(payload.PullRequest.Number, 10)}
	switch {
	case request.Headers["X-Github-Event"] == "pull_request_review" && payload.Action == "submitted":
		if payload.Review.ID <= 0 {
			return "", nil, problem.ErrInvalid
		}
		event.Type, event.Key, event.OccurredAt = ReviewSubmitted, reviewEventKey(payload.Review.ID), payload.Review.SubmittedAt
	case request.Headers["X-Github-Event"] == "pull_request" && payload.Action == "closed" && payload.PullRequest.Merged:
		event.Type, event.Key, event.OccurredAt = PullRequestMerged, mergedEventKey(event.RepositoryID, event.ResourceID), payload.PullRequest.MergedAt
	case request.Headers["X-Github-Event"] == "pull_request" && (payload.Action == "opened" || payload.Action == "reopened" || payload.Action == "synchronize"):
		event.Type, event.Key, event.OccurredAt = "pull_request.discovered", discoveredEventKey(event.RepositoryID, event.ResourceID, payload.PullRequest.Head.SHA, payload.PullRequest.UpdatedAt), payload.PullRequest.UpdatedAt
	default:
		return deliveryID, nil, nil
	}
	if payload.PullRequest.Number <= 0 || event.OccurredAt.IsZero() {
		return "", nil, problem.ErrInvalid
	}
	if !repositoryName.MatchString(payload.Repository.FullName) {
		return "", nil, problem.ErrInvalid
	}
	event.ReferenceURL = "https://github.com/" + payload.Repository.FullName + "/pull/" + event.ResourceID
	return deliveryID, []trigger.Event{event}, nil
}
