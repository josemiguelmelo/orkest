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
	stageintegrationmodel "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration/model"
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

var _ integration.WorkflowStageIntegrationProvider = GitHubPRIntegration{}

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

func (GitHubPRIntegration) Actions() []stageintegrationmodel.ActionDefinition {
	return []stageintegrationmodel.ActionDefinition{
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

func parseReviewParameters(raw json.RawMessage) (reviewParameters, error) {
	var p reviewParameters
	if err := decode(raw, &p); err != nil {
		return p, err
	}
	if p.ArtifactInputName == "" || len(p.ReviewStates) == 0 || len(p.ReviewStates) > 3 {
		return p, problem.ErrInvalid
	}
	seen := map[ReviewState]bool{}
	for _, state := range p.ReviewStates {
		if (state != ReviewStateApproved && state != ReviewStateChangesRequested && state != ReviewStateCommented) || seen[state] {
			return p, problem.ErrInvalid
		}
		seen[state] = true
	}
	return p, nil
}

func parseMergeParameters(raw json.RawMessage) (mergeParameters, error) {
	var p mergeParameters
	if err := decode(raw, &p); err != nil {
		return p, err
	}
	if p.ArtifactInputName == "" || (p.MergePolicy != "" && p.MergePolicy != MergePolicyAll && p.MergePolicy != MergePolicyAny) {
		return p, problem.ErrInvalid
	}
	return p, nil
}

func (GitHubPRIntegration) ValidateParameters(action string, raw json.RawMessage) error {
	switch ActionID(action) {
	case ActionReviewAdded:
		_, err := parseReviewParameters(raw)
		return err
	case ActionPRMerged:
		_, err := parseMergeParameters(raw)
		return err
	default:
		return problem.ErrInvalid
	}
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

func (p GitHubPRIntegration) HandleCallback(ctx context.Context, in stageintegrationmodel.CallbackInput) (stageintegrationmodel.CallbackResult, error) {
	if err := ctx.Err(); err != nil {
		return stageintegrationmodel.CallbackResult{}, err
	}
	signature := header(in.Request.Headers, "X-Hub-Signature-256")
	supplied, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	mac := hmac.New(sha256.New, []byte(in.Secret))
	_, _ = mac.Write(in.Request.Body)
	if in.Secret == "" || !strings.HasPrefix(signature, "sha256=") || err != nil || !hmac.Equal(supplied, mac.Sum(nil)) {
		return stageintegrationmodel.CallbackResult{}, integration.ErrUnauthenticated
	}
	if in.Request.Method != "POST" {
		return stageintegrationmodel.CallbackResult{}, problem.ErrInvalid
	}
	if err := p.ValidateConfig(in.Config); err != nil {
		return stageintegrationmodel.CallbackResult{}, err
	}
	kind := WebhookEventKind(header(in.Request.Headers, "X-GitHub-Event"))
	if kind != WebhookEventPullRequestReview && kind != WebhookEventPullRequest {
		return stageintegrationmodel.CallbackResult{Ignored: true}, nil
	}
	var payload struct {
		Action      WebhookAction                `json:"action"`
		PullRequest *githubsdk.PullRequest       `json:"pull_request"`
		Review      *githubsdk.PullRequestReview `json:"review"`
		Repository  *githubsdk.Repository        `json:"repository"`
	}
	if bytes.Equal(bytes.TrimSpace(in.Request.Body), []byte("null")) {
		return stageintegrationmodel.CallbackResult{}, problem.ErrInvalid
	}
	if err := json.Unmarshal(in.Request.Body, &payload); err != nil {
		return stageintegrationmodel.CallbackResult{}, problem.ErrInvalid
	}
	if kind == WebhookEventPullRequestReview && payload.Action != WebhookActionSubmitted || kind == WebhookEventPullRequest && payload.Action != WebhookActionClosed {
		return stageintegrationmodel.CallbackResult{Ignored: true}, nil
	}
	if kind == WebhookEventPullRequest && !payload.PullRequest.GetMerged() {
		return stageintegrationmodel.CallbackResult{Ignored: true}, nil
	}
	subject, err := subjectFromPR(payload.PullRequest)
	if err != nil {
		return stageintegrationmodel.CallbackResult{}, err
	}
	if payload.Repository.GetID() <= 0 || strconv.FormatInt(payload.Repository.GetID(), 10) != subject.RepositoryID {
		return stageintegrationmodel.CallbackResult{}, problem.ErrInvalid
	}
	var event stageintegrationmodel.ProviderEvent
	if kind == WebhookEventPullRequestReview {
		event, err = reviewEvent(subject, payload.Review)
	} else {
		event, err = mergeEvent(subject, payload.PullRequest)
	}
	if err != nil {
		return stageintegrationmodel.CallbackResult{}, err
	}
	delivery := header(in.Request.Headers, "X-GitHub-Delivery")
	if delivery == "" || len(delivery) > 200 {
		return stageintegrationmodel.CallbackResult{}, problem.ErrInvalid
	}
	return stageintegrationmodel.CallbackResult{DeliveryID: delivery, Events: []stageintegrationmodel.ProviderEvent{event}}, nil
}

func subjectFromPR(pr *githubsdk.PullRequest) (stageintegrationmodel.Subject, error) {
	if pr.GetID() <= 0 || pr.GetNumber() <= 0 || pr.GetBase().GetRepo().GetID() <= 0 || pr.GetHead().GetSHA() == "" {
		return stageintegrationmodel.Subject{}, problem.ErrInvalid
	}
	owner, repo, number, err := parseURL(pr.GetHTMLURL())
	if err != nil || number != pr.GetNumber() {
		return stageintegrationmodel.Subject{}, problem.ErrInvalid
	}
	return stageintegrationmodel.Subject{Kind: string(SubjectKindCodeReview), ID: strconv.FormatInt(pr.GetID(), 10), RepositoryID: strconv.FormatInt(pr.GetBase().GetRepo().GetID(), 10), URL: fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, number), HeadSHA: pr.GetHead().GetSHA(), Merged: pr.GetMerged()}, nil
}

func reviewEvent(subject stageintegrationmodel.Subject, review *githubsdk.PullRequestReview) (stageintegrationmodel.ProviderEvent, error) {
	state := ReviewState(strings.ToLower(review.GetState()))
	at := review.GetSubmittedAt().Time
	if review.GetID() <= 0 || at.IsZero() || review.GetCommitID() == "" || (state != ReviewStateApproved && state != ReviewStateChangesRequested && state != ReviewStateCommented) {
		return stageintegrationmodel.ProviderEvent{}, problem.ErrInvalid
	}
	facts, _ := json.Marshal(eventFacts{ReviewID: review.GetID(), ReviewState: state, CommitSHA: review.GetCommitID(), ReviewURL: subject.URL + "#pullrequestreview-" + strconv.FormatInt(review.GetID(), 10)})
	return stageintegrationmodel.ProviderEvent{ID: "review:" + subject.RepositoryID + ":" + subject.ID + ":" + strconv.FormatInt(review.GetID(), 10), ActionID: string(ActionReviewAdded), Subject: subject, OccurredAt: at, Facts: facts}, nil
}

func mergeEvent(subject stageintegrationmodel.Subject, pr *githubsdk.PullRequest) (stageintegrationmodel.ProviderEvent, error) {
	at := pr.GetMergedAt().Time
	if !subject.Merged || at.IsZero() {
		return stageintegrationmodel.ProviderEvent{}, problem.ErrInvalid
	}
	facts, _ := json.Marshal(eventFacts{CommitSHA: pr.GetMergeCommitSHA()})
	return stageintegrationmodel.ProviderEvent{ID: "merge:" + subject.RepositoryID + ":" + subject.ID, ActionID: string(ActionPRMerged), Subject: subject, OccurredAt: at, Facts: facts}, nil
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

func (p GitHubPRIntegration) ValidateConnection(ctx context.Context, config json.RawMessage, token string, repositoryIDs []string) error {
	if err := p.ValidateConfig(config); err != nil {
		return err
	}
	client, err := p.client(token)
	if err != nil {
		return err
	}
	if len(repositoryIDs) == 0 {
		return problem.ErrInvalid
	}
	for _, rawID := range repositoryIDs {
		id, parseErr := strconv.ParseInt(rawID, 10, 64)
		if parseErr != nil || id <= 0 {
			return problem.ErrInvalid
		}
		repository, _, readErr := client.Repositories.GetByID(ctx, id)
		if readErr != nil || repository.GetID() != id || repository.GetOwner().GetLogin() == "" || repository.GetName() == "" {
			return errors.New("GitHub repository access validation failed")
		}
		// Repository metadata alone does not establish pull-request read access.
		if _, _, readErr = client.PullRequests.List(ctx, repository.GetOwner().GetLogin(), repository.GetName(), &githubsdk.PullRequestListOptions{ListOptions: githubsdk.ListOptions{PerPage: 1}}); readErr != nil {
			return errors.New("GitHub pull request access validation failed")
		}
	}
	return nil
}

func (p GitHubPRIntegration) ResolveSubject(ctx context.Context, config json.RawMessage, token, rawURL string) (stageintegrationmodel.Subject, error) {
	if err := p.ValidateConfig(config); err != nil {
		return stageintegrationmodel.Subject{}, err
	}
	owner, repo, number, err := parseURL(rawURL)
	if err != nil {
		return stageintegrationmodel.Subject{}, err
	}
	client, err := p.client(token)
	if err != nil {
		return stageintegrationmodel.Subject{}, err
	}
	pr, _, err := client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return stageintegrationmodel.Subject{}, errors.New("GitHub pull request verification failed")
	}
	return subjectFromPR(pr)
}

func (p GitHubPRIntegration) Reconcile(ctx context.Context, config json.RawMessage, token, action string, subject stageintegrationmodel.Subject, since time.Time) (stageintegrationmodel.Reconciliation, error) {
	if ActionID(action) != ActionPRMerged && ActionID(action) != ActionReviewAdded {
		return stageintegrationmodel.Reconciliation{}, problem.ErrInvalid
	}
	if err := p.ValidateConfig(config); err != nil {
		return stageintegrationmodel.Reconciliation{}, err
	}
	owner, repo, number, err := parseURL(subject.URL)
	if err != nil {
		return stageintegrationmodel.Reconciliation{}, err
	}
	client, err := p.client(token)
	if err != nil {
		return stageintegrationmodel.Reconciliation{}, err
	}
	pr, _, err := client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return stageintegrationmodel.Reconciliation{}, errors.New("GitHub pull request reconciliation failed")
	}
	verified, err := subjectFromPR(pr)
	if err != nil {
		return stageintegrationmodel.Reconciliation{}, err
	}
	if verified.ID != subject.ID || verified.RepositoryID != subject.RepositoryID {
		return stageintegrationmodel.Reconciliation{}, problem.ErrConflict
	}
	events := []stageintegrationmodel.ProviderEvent{}
	if ActionID(action) == ActionPRMerged && verified.Merged {
		event, e := mergeEvent(verified, pr)
		if e != nil {
			return stageintegrationmodel.Reconciliation{}, e
		}
		if event.OccurredAt.After(since) {
			events = append(events, event)
		}
	}
	if ActionID(action) == ActionPRMerged {
		return stageintegrationmodel.Reconciliation{Subject: verified, Events: events}, nil
	}
	options := &githubsdk.ListOptions{PerPage: 100}
	for {
		reviews, response, e := client.PullRequests.ListReviews(ctx, owner, repo, number, options)
		if e != nil {
			return stageintegrationmodel.Reconciliation{}, errors.New("GitHub review reconciliation failed")
		}
		for _, review := range reviews {
			state := ReviewState(strings.ToLower(review.GetState()))
			if state == ReviewStatePending || state == ReviewStateDismissed {
				continue
			}
			event, e := reviewEvent(verified, review)
			if e != nil {
				return stageintegrationmodel.Reconciliation{}, e
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
	return stageintegrationmodel.Reconciliation{Subject: verified, Events: events}, nil
}

func (p GitHubPRIntegration) MatchSubjects(action string, parameters json.RawMessage, event stageintegrationmodel.ProviderEvent, subjects []stageintegrationmodel.Subject) (stageintegrationmodel.SubjectMatch, error) {
	if ActionID(action) == ActionReviewAdded {
		params, err := parseReviewParameters(parameters)
		if err != nil {
			return stageintegrationmodel.SubjectMatch{}, err
		}
		if event.ActionID != action {
			return stageintegrationmodel.SubjectMatch{}, nil
		}
		var facts eventFacts
		if err := json.Unmarshal(event.Facts, &facts); err != nil {
			return stageintegrationmodel.SubjectMatch{}, problem.ErrInvalid
		}
		if (params.RequireCurrentHead == nil || *params.RequireCurrentHead) && (facts.CommitSHA == "" || facts.CommitSHA != event.Subject.HeadSHA) {
			return stageintegrationmodel.SubjectMatch{}, nil
		}
		for _, state := range params.ReviewStates {
			if state == facts.ReviewState {
				return stageintegrationmodel.SubjectMatch{Matched: true}, nil
			}
		}
		return stageintegrationmodel.SubjectMatch{}, nil
	}
	if ActionID(action) != ActionPRMerged {
		return stageintegrationmodel.SubjectMatch{}, problem.ErrInvalid
	}
	params, err := parseMergeParameters(parameters)
	if err != nil {
		return stageintegrationmodel.SubjectMatch{}, err
	}
	if event.ActionID != action || !event.Subject.Merged {
		return stageintegrationmodel.SubjectMatch{}, nil
	}
	if params.MergePolicy == MergePolicyAny {
		return stageintegrationmodel.SubjectMatch{Matched: true}, nil
	}
	if len(subjects) == 0 {
		return stageintegrationmodel.SubjectMatch{}, nil
	}
	for _, subject := range subjects {
		if !subject.Merged {
			return stageintegrationmodel.SubjectMatch{}, nil
		}
	}
	return stageintegrationmodel.SubjectMatch{Matched: true, OncePerObservation: true}, nil
}
