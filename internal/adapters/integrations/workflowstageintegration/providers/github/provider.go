package github

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	githubsdk "github.com/google/go-github/v93/github"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	integration "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration"
)

type (
	ActionID         string
	ProviderID       string
	WebhookEventKind string
	WebhookAction    string
	SubjectKind      string
	ReviewState      string
	MergePolicy      string
)

const (
	ProviderName ProviderID = "github_pr"

	ActionReviewAdded ActionID = "review_added"
	ActionPRMerged    ActionID = "pr_merged"

	WebhookEventPullRequest       WebhookEventKind = "pull_request"
	WebhookEventPullRequestReview WebhookEventKind = "pull_request_review"
	WebhookActionClosed           WebhookAction    = "closed"
	WebhookActionSubmitted        WebhookAction    = "submitted"

	SubjectKindCodeReview SubjectKind = "code_review"

	ReviewStateApproved         ReviewState = "approved"
	ReviewStateChangesRequested ReviewState = "changes_requested"
	ReviewStateCommented        ReviewState = "commented"
	ReviewStatePending          ReviewState = "pending"
	ReviewStateDismissed        ReviewState = "dismissed"

	MergePolicyAll MergePolicy = "all"
	MergePolicyAny MergePolicy = "any"
)

type GitHubPRIntegration struct {
	Client     *http.Client
	APIBaseURL string
}

var (
	_ integration.WorkflowStageIntegration = GitHubPRIntegration{}
	_ integration.SubjectProvider          = GitHubPRIntegration{}
)

func NewProvider() GitHubPRIntegration {
	return GitHubPRIntegration{Client: &http.Client{Timeout: 20 * time.Second}}
}
func (GitHubPRIntegration) Provider() string       { return string(ProviderName) }
func (GitHubPRIntegration) CodingProvider() string { return "github" }
func (GitHubPRIntegration) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
}

func (GitHubPRIntegration) ValidateConfig(raw json.RawMessage) error {
	var value struct{}
	return decode(raw, &value)
}

func (GitHubPRIntegration) Actions() []integration.ActionDefinition {
	return []integration.ActionDefinition{
		{ID: string(ActionReviewAdded), Name: "PR review added", ParametersSchema: reviewActionSchema()},
		{ID: string(ActionPRMerged), Name: "PR merged", ParametersSchema: mergeActionSchema()},
	}
}

func reviewActionSchema() json.RawMessage {
	return marshalSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"artifactInputName": map[string]any{"type": "string", "title": "PR artifact input"},
			"reviewStates": map[string]any{
				"type": "array", "title": "Review states",
				"items":    map[string]any{"type": "string", "enum": []ReviewState{ReviewStateApproved, ReviewStateChangesRequested, ReviewStateCommented}},
				"minItems": 1, "uniqueItems": true, "default": []ReviewState{ReviewStateChangesRequested},
			},
			"requireCurrentHead": map[string]any{"type": "boolean", "title": "Require current head commit", "default": true},
		},
		"required": []string{"artifactInputName", "reviewStates"}, "additionalProperties": false,
	})
}

func mergeActionSchema() json.RawMessage {
	return marshalSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"artifactInputName": map[string]any{"type": "string", "title": "PR artifact input"},
			"mergePolicy":       map[string]any{"type": "string", "title": "Merge policy", "enum": []MergePolicy{MergePolicyAll, MergePolicyAny}, "default": MergePolicyAll},
		},
		"required": []string{"artifactInputName"}, "additionalProperties": false,
	})
}

func marshalSchema(value any) json.RawMessage {
	result, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return result
}

type reviewParameters struct {
	ArtifactInputName  string        `json:"artifactInputName"`
	ReviewStates       []ReviewState `json:"reviewStates"`
	RequireCurrentHead *bool         `json:"requireCurrentHead"`
}
type mergeParameters struct {
	ArtifactInputName string      `json:"artifactInputName"`
	MergePolicy       MergePolicy `json:"mergePolicy"`
}
type eventFacts struct {
	ReviewID    int64       `json:"reviewId,omitempty"`
	ReviewState ReviewState `json:"reviewState,omitempty"`
	CommitSHA   string      `json:"commitSha,omitempty"`
	ReviewURL   string      `json:"reviewUrl,omitempty"`
}

func (GitHubPRIntegration) ValidateParameters(action string, raw json.RawMessage) error {
	switch ActionID(action) {
	case ActionReviewAdded:
		var p reviewParameters
		if err := decode(raw, &p); err != nil {
			return err
		}
		if p.ArtifactInputName == "" || len(p.ReviewStates) == 0 || len(p.ReviewStates) > 3 {
			return problem.ErrInvalid
		}
		seen := map[ReviewState]bool{}
		for _, s := range p.ReviewStates {
			if (s != ReviewStateApproved && s != ReviewStateChangesRequested && s != ReviewStateCommented) || seen[s] {
				return problem.ErrInvalid
			}
			seen[s] = true
		}
	case ActionPRMerged:
		var p mergeParameters
		if err := decode(raw, &p); err != nil {
			return err
		}
		if p.ArtifactInputName == "" || (p.MergePolicy != "" && p.MergePolicy != MergePolicyAll && p.MergePolicy != MergePolicyAny) {
			return problem.ErrInvalid
		}
	default:
		return problem.ErrInvalid
	}
	return nil
}

func decode(raw []byte, target any) error {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return problem.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return problem.ErrInvalid
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return problem.ErrInvalid
	}
	return nil
}

func header(headers map[string][]string, key string) string {
	value := ""
	found := false
	for k, values := range headers {
		if !strings.EqualFold(k, key) {
			continue
		}
		if found || len(values) != 1 {
			return ""
		}
		found, value = true, values[0]
	}
	return value
}

func (p GitHubPRIntegration) HandleCallback(ctx context.Context, in integration.CallbackInput) (integration.CallbackResult, error) {
	if err := ctx.Err(); err != nil {
		return integration.CallbackResult{}, err
	}
	signature := header(in.Request.Headers, "X-Hub-Signature-256")
	supplied, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	mac := hmac.New(sha256.New, []byte(in.Secret))
	_, _ = mac.Write(in.Request.Body)
	if in.Secret == "" || !strings.HasPrefix(signature, "sha256=") || err != nil || !hmac.Equal(supplied, mac.Sum(nil)) {
		return integration.CallbackResult{}, integration.ErrUnauthenticated
	}
	if in.Request.Method != "POST" {
		return integration.CallbackResult{}, problem.ErrInvalid
	}
	if err := p.ValidateConfig(in.Config); err != nil {
		return integration.CallbackResult{}, err
	}
	kind := WebhookEventKind(header(in.Request.Headers, "X-GitHub-Event"))
	if kind != WebhookEventPullRequestReview && kind != WebhookEventPullRequest {
		return integration.CallbackResult{Ignored: true}, nil
	}
	var payload struct {
		Action      WebhookAction                `json:"action"`
		PullRequest *githubsdk.PullRequest       `json:"pull_request"`
		Review      *githubsdk.PullRequestReview `json:"review"`
		Repository  *githubsdk.Repository        `json:"repository"`
	}
	if bytes.Equal(bytes.TrimSpace(in.Request.Body), []byte("null")) {
		return integration.CallbackResult{}, problem.ErrInvalid
	}
	if err := json.Unmarshal(in.Request.Body, &payload); err != nil {
		return integration.CallbackResult{}, problem.ErrInvalid
	}
	if kind == WebhookEventPullRequestReview && payload.Action != WebhookActionSubmitted || kind == WebhookEventPullRequest && payload.Action != WebhookActionClosed {
		return integration.CallbackResult{Ignored: true}, nil
	}
	if kind == WebhookEventPullRequest && !payload.PullRequest.GetMerged() {
		return integration.CallbackResult{Ignored: true}, nil
	}
	subject, err := subjectFromPR(payload.PullRequest)
	if err != nil {
		return integration.CallbackResult{}, err
	}
	if payload.Repository.GetID() <= 0 || strconv.FormatInt(payload.Repository.GetID(), 10) != subject.RepositoryID {
		return integration.CallbackResult{}, problem.ErrInvalid
	}
	var event integration.ProviderEvent
	if kind == WebhookEventPullRequestReview {
		event, err = reviewEvent(subject, payload.Review)
	} else {
		event, err = mergeEvent(subject, payload.PullRequest)
	}
	if err != nil {
		return integration.CallbackResult{}, err
	}
	delivery := header(in.Request.Headers, "X-GitHub-Delivery")
	if delivery == "" || len(delivery) > 200 {
		return integration.CallbackResult{}, problem.ErrInvalid
	}
	return integration.CallbackResult{DeliveryID: delivery, Events: []integration.ProviderEvent{event}}, nil
}

func subjectFromPR(pr *githubsdk.PullRequest) (integration.Subject, error) {
	if pr.GetID() <= 0 || pr.GetNumber() <= 0 || pr.GetBase().GetRepo().GetID() <= 0 || pr.GetHead().GetSHA() == "" {
		return integration.Subject{}, problem.ErrInvalid
	}
	owner, repo, number, err := parseURL(pr.GetHTMLURL())
	if err != nil || number != pr.GetNumber() {
		return integration.Subject{}, problem.ErrInvalid
	}
	return integration.Subject{Kind: string(SubjectKindCodeReview), ID: strconv.FormatInt(pr.GetID(), 10), RepositoryID: strconv.FormatInt(pr.GetBase().GetRepo().GetID(), 10), URL: fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, number), HeadSHA: pr.GetHead().GetSHA(), Merged: pr.GetMerged()}, nil
}

func reviewEvent(subject integration.Subject, review *githubsdk.PullRequestReview) (integration.ProviderEvent, error) {
	state := ReviewState(strings.ToLower(review.GetState()))
	at := review.GetSubmittedAt().Time
	if review.GetID() <= 0 || at.IsZero() || review.GetCommitID() == "" || (state != ReviewStateApproved && state != ReviewStateChangesRequested && state != ReviewStateCommented) {
		return integration.ProviderEvent{}, problem.ErrInvalid
	}
	facts, _ := json.Marshal(eventFacts{ReviewID: review.GetID(), ReviewState: state, CommitSHA: review.GetCommitID(), ReviewURL: subject.URL + "#pullrequestreview-" + strconv.FormatInt(review.GetID(), 10)})
	return integration.ProviderEvent{ID: "review:" + subject.RepositoryID + ":" + subject.ID + ":" + strconv.FormatInt(review.GetID(), 10), ActionID: string(ActionReviewAdded), Subject: subject, OccurredAt: at, Facts: facts}, nil
}

func mergeEvent(subject integration.Subject, pr *githubsdk.PullRequest) (integration.ProviderEvent, error) {
	at := pr.GetMergedAt().Time
	if !subject.Merged || at.IsZero() {
		return integration.ProviderEvent{}, problem.ErrInvalid
	}
	facts, _ := json.Marshal(eventFacts{CommitSHA: pr.GetMergeCommitSHA()})
	return integration.ProviderEvent{ID: "merge:" + subject.RepositoryID + ":" + subject.ID, ActionID: string(ActionPRMerged), Subject: subject, OccurredAt: at, Facts: facts}, nil
}

func (p GitHubPRIntegration) Matches(action string, parameters json.RawMessage, event integration.ProviderEvent) (bool, error) {
	if err := p.ValidateParameters(action, parameters); err != nil {
		return false, err
	}
	if event.ActionID != action {
		return false, nil
	}
	if ActionID(action) == ActionPRMerged {
		return event.Subject.Merged, nil
	}
	var params reviewParameters
	if err := decode(parameters, &params); err != nil {
		return false, err
	}
	var facts eventFacts
	if err := json.Unmarshal(event.Facts, &facts); err != nil {
		return false, problem.ErrInvalid
	}
	if params.RequireCurrentHead == nil || *params.RequireCurrentHead {
		if facts.CommitSHA == "" || facts.CommitSHA != event.Subject.HeadSHA {
			return false, nil
		}
	}
	for _, state := range params.ReviewStates {
		if state == facts.ReviewState {
			return true, nil
		}
	}
	return false, nil
}

func parseURL(raw string) (string, string, int, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", "", 0, problem.ErrInvalid
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 4 || parts[0] == "" || parts[1] == "" || parts[2] != "pull" || strings.ContainsAny(parts[0]+parts[1], "%\\") {
		return "", "", 0, problem.ErrInvalid
	}
	number, err := strconv.Atoi(parts[3])
	if err != nil || number <= 0 {
		return "", "", 0, problem.ErrInvalid
	}
	return parts[0], parts[1], number, nil
}

func (p GitHubPRIntegration) client(token string) (*githubsdk.Client, error) {
	httpClient := p.Client
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	} else if httpClient.Timeout == 0 {
		bounded := *httpClient
		bounded.Timeout = 20 * time.Second
		httpClient = &bounded
	}
	options := []githubsdk.ClientOptionsFunc{githubsdk.WithHTTPClient(httpClient), githubsdk.WithAuthToken(token)}
	if p.APIBaseURL != "" {
		base := strings.TrimRight(p.APIBaseURL, "/") + "/"
		options = append(options, githubsdk.WithURLs(&base, nil))
	}
	return githubsdk.NewClient(options...)
}

func (p GitHubPRIntegration) ValidateConnection(ctx context.Context, config json.RawMessage, token string) error {
	if err := p.ValidateConfig(config); err != nil {
		return err
	}
	client, err := p.client(token)
	if err != nil {
		return err
	}
	if _, _, err := client.Users.Get(ctx, ""); err != nil {
		return errors.New("GitHub connection validation failed")
	}
	return nil
}

func (p GitHubPRIntegration) ResolveSubject(ctx context.Context, config json.RawMessage, token, rawURL string) (integration.Subject, error) {
	if err := p.ValidateConfig(config); err != nil {
		return integration.Subject{}, err
	}
	owner, repo, number, err := parseURL(rawURL)
	if err != nil {
		return integration.Subject{}, err
	}
	client, err := p.client(token)
	if err != nil {
		return integration.Subject{}, err
	}
	pr, _, err := client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return integration.Subject{}, errors.New("GitHub pull request verification failed")
	}
	return subjectFromPR(pr)
}

func (p GitHubPRIntegration) Reconcile(ctx context.Context, config json.RawMessage, token string, subject integration.Subject, since time.Time) ([]integration.ProviderEvent, error) {
	if err := p.ValidateConfig(config); err != nil {
		return nil, err
	}
	owner, repo, number, err := parseURL(subject.URL)
	if err != nil {
		return nil, err
	}
	client, err := p.client(token)
	if err != nil {
		return nil, err
	}
	pr, _, err := client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, errors.New("GitHub pull request reconciliation failed")
	}
	verified, err := subjectFromPR(pr)
	if err != nil {
		return nil, err
	}
	if verified.ID != subject.ID || verified.RepositoryID != subject.RepositoryID {
		return nil, problem.ErrConflict
	}
	events := []integration.ProviderEvent{}
	if verified.Merged {
		event, e := mergeEvent(verified, pr)
		if e != nil {
			return nil, e
		}
		if event.OccurredAt.After(since) {
			events = append(events, event)
		}
	}
	options := &githubsdk.ListOptions{PerPage: 100}
	for {
		reviews, response, e := client.PullRequests.ListReviews(ctx, owner, repo, number, options)
		if e != nil {
			return nil, errors.New("GitHub review reconciliation failed")
		}
		for _, review := range reviews {
			state := ReviewState(strings.ToLower(review.GetState()))
			if state == ReviewStatePending || state == ReviewStateDismissed {
				continue
			}
			event, e := reviewEvent(verified, review)
			if e != nil {
				return nil, e
			}
			if event.OccurredAt.After(since) {
				events = append(events, event)
			}
		}
		if response.NextPage == 0 {
			break
		}
		options.Page = response.NextPage
	}
	return events, nil
}

func (p GitHubPRIntegration) MatchSubjects(action string, parameters json.RawMessage, event integration.ProviderEvent, subjects []integration.Subject) (integration.SubjectMatch, error) {
	matched, err := p.Matches(action, parameters, event)
	if err != nil || !matched {
		return integration.SubjectMatch{}, err
	}
	if ActionID(action) != ActionPRMerged {
		return integration.SubjectMatch{Matched: true}, nil
	}
	var params mergeParameters
	if err := decode(parameters, &params); err != nil {
		return integration.SubjectMatch{}, err
	}
	if params.MergePolicy == MergePolicyAny {
		return integration.SubjectMatch{Matched: true}, nil
	}
	if len(subjects) == 0 {
		return integration.SubjectMatch{}, nil
	}
	for _, subject := range subjects {
		if !subject.Merged {
			return integration.SubjectMatch{}, nil
		}
	}
	return integration.SubjectMatch{Matched: true, OncePerObservation: true}, nil
}
