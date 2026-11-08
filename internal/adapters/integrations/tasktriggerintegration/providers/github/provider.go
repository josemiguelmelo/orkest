package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
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
	RepositoryID string `json:"repositoryId" jsonschema:"title=Repository ID,description=GitHub numeric repository ID,minLength=1"`
}

func NewGitHubProvider() *GitHubProvider { return &GitHubProvider{} }

func (*GitHubProvider) Descriptor() trigger.ProviderDescriptor {
	return trigger.ProviderDescriptor{ID: "github", Name: "GitHub", ConfigSchema: taskintegration.GenerateProviderConfigSchema(configuration{}, "GitHub webhook"), Events: []string{ReviewSubmitted, PullRequestMerged}}
}

func (*GitHubProvider) ValidateConfig(raw json.RawMessage) error {
	c, err := taskintegration.DecodeProviderConfig[configuration](raw)
	if err != nil || !positiveID(c.RepositoryID) {
		return problem.ErrInvalid
	}
	return nil
}

func (*GitHubProvider) ValidateResource(raw json.RawMessage, link trigger.ResourceLink) error {
	var c configuration
	if json.Unmarshal(raw, &c) != nil || link.RepositoryID != c.RepositoryID || !positiveID(link.ResourceID) {
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
	var config configuration
	if json.Unmarshal(raw, &config) != nil {
		return "", nil, problem.ErrInvalid
	}
	if request.Headers["X-Github-Event"] == "ping" {
		return deliveryID, nil, nil
	}
	if strconv.FormatInt(payload.Repository.ID, 10) != config.RepositoryID {
		return "", nil, problem.ErrForbidden
	}
	event := trigger.Event{RepositoryID: config.RepositoryID, ResourceID: strconv.FormatInt(payload.PullRequest.Number, 10)}
	switch {
	case request.Headers["X-Github-Event"] == "pull_request_review" && payload.Action == "submitted":
		if payload.Review.ID <= 0 {
			return "", nil, problem.ErrInvalid
		}
		event.Type, event.Key, event.OccurredAt = ReviewSubmitted, "review:"+strconv.FormatInt(payload.Review.ID, 10), payload.Review.SubmittedAt
	case request.Headers["X-Github-Event"] == "pull_request" && payload.Action == "closed" && payload.PullRequest.Merged:
		event.Type, event.Key, event.OccurredAt = PullRequestMerged, "merged:"+event.RepositoryID+":"+event.ResourceID, payload.PullRequest.MergedAt
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
