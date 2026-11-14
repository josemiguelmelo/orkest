package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	integration "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration"
	stageintegrationmodel "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration/model"
)

const fixtureSecret = "webhook-secret-fixture-long-enough"

func callbackInput(body, kind string) stageintegrationmodel.CallbackInput {
	mac := hmac.New(sha256.New, []byte(fixtureSecret))
	_, _ = mac.Write([]byte(body))
	return stageintegrationmodel.CallbackInput{Config: json.RawMessage(`{}`), Secret: fixtureSecret, Request: stageintegrationmodel.CallbackRequest{Method: "POST", Body: []byte(body), Headers: map[string][]string{"x-hub-signature-256": {"sha256=" + hex.EncodeToString(mac.Sum(nil))}, "x-github-event": {kind}, "x-github-delivery": {"delivery-1"}}}}
}

func prFixture(merged bool) string {
	mergedTime := "null"
	if merged {
		mergedTime = `"2026-11-11T10:03:00Z"`
	}
	return fmt.Sprintf(`{"id":50,"number":42,"html_url":"https://github.com/acme/backend/pull/42","base":{"repo":{"id":123}},"head":{"sha":"head-commit","repo":{"id":999}},"merged":%t,"merged_at":%s,"merge_commit_sha":"merge-commit"}`, merged, mergedTime)
}

func reviewFixture(state, commit string, id int) string {
	return fmt.Sprintf(`{"id":%d,"submitted_at":"2026-11-11T10:02:00Z","state":%q,"commit_id":%q,"body":"untrusted sensitive review text"}`, id, state, commit)
}

func reviewPayload(state, commit string, id int) string {
	return fmt.Sprintf(`{"action":"submitted","repository":{"id":123},"pull_request":%s,"review":%s}`, prFixture(false), reviewFixture(state, commit, id))
}

func TestCallbackAuthenticatesOriginalBytesAndNormalizesReview(t *testing.T) {
	p := NewProvider()
	in := callbackInput(reviewPayload("changes_requested", "head-commit", 71), "pull_request_review")
	result, err := p.HandleCallback(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeliveryID != "delivery-1" || len(result.Events) != 1 {
		t.Fatalf("result=%+v", result)
	}
	event := result.Events[0]
	if event.ID != "review:123:50:71" || event.ActionID != string(ActionReviewAdded) || event.Subject.Kind != string(SubjectKindCodeReview) || event.Subject.RepositoryID != "123" || event.Subject.ID != "50" {
		t.Fatalf("event=%+v", event)
	}
	if strings.Contains(string(event.Facts), "sensitive") {
		t.Fatal("review body retained in normalized facts")
	}
	params := json.RawMessage(`{"artifactInputName":"prs","reviewStates":["changes_requested"]}`)
	matched, err := p.MatchSubjects(string(ActionReviewAdded), params, event, nil)
	if err != nil || !matched.Matched {
		t.Fatalf("matched=%+v err=%v", matched, err)
	}
	in.Request.Body = append(in.Request.Body, ' ')
	if _, err := p.HandleCallback(context.Background(), in); !errors.Is(err, integration.ErrUnauthenticated) {
		t.Fatalf("mutated body accepted: %v", err)
	}
	in.Secret = "wrong"
	if _, err := p.HandleCallback(context.Background(), in); !errors.Is(err, integration.ErrUnauthenticated) {
		t.Fatalf("wrong key accepted: %v", err)
	}
}

func TestCallbackIgnoresUnsupportedActionsOnlyAfterAuthentication(t *testing.T) {
	p := NewProvider()
	for _, tc := range []struct{ kind, body string }{
		{"ping", `{}`},
		{"issue_comment", `{}`},
		{"pull_request_review", `{"action":"edited"}`},
		{"pull_request", `{"action":"closed","pull_request":{"merged":false}}`},
	} {
		t.Run(tc.kind+tc.body, func(t *testing.T) {
			in := callbackInput(tc.body, tc.kind)
			result, err := p.HandleCallback(context.Background(), in)
			if err != nil || !result.Ignored {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			in.Request.Headers["x-hub-signature-256"] = []string{"sha256=invalid"}
			if _, err := p.HandleCallback(context.Background(), in); !errors.Is(err, integration.ErrUnauthenticated) {
				t.Fatalf("unsupported event bypassed auth: %v", err)
			}
		})
	}
}

func TestCallbackRejectsMalformedIdentityAndRecognizesMerge(t *testing.T) {
	p := NewProvider()
	for _, body := range []string{`null`, `{`, `{"action":"submitted"}`, strings.ReplaceAll(reviewPayload("approved", "head-commit", 71), `"repository":{"id":123}`, `"repository":{"id":999}`)} {
		if _, err := p.HandleCallback(context.Background(), callbackInput(body, "pull_request_review")); !errors.Is(err, problem.ErrInvalid) {
			t.Fatalf("accepted malformed body %s: %v", body, err)
		}
	}
	body := fmt.Sprintf(`{"action":"closed","repository":{"id":123},"pull_request":%s}`, prFixture(true))
	result, err := p.HandleCallback(context.Background(), callbackInput(body, "pull_request"))
	if err != nil || len(result.Events) != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if result.Events[0].ID != "merge:123:50" || result.Events[0].ActionID != string(ActionPRMerged) {
		t.Fatalf("event=%+v", result.Events[0])
	}
}

func TestTypedParameterValidationAndHeadFilters(t *testing.T) {
	p := NewProvider()
	for _, tc := range []struct{ action, params string }{
		{"unknown", `{}`},
		{"review_added", `{"artifactInputName":"prs","reviewStates":["invalid"]}`},
		{"review_added", `{"artifactInputName":"prs","reviewStates":["approved","approved"]}`},
		{"review_added", `{"artifactInputName":"prs","reviewStates":["approved"],"script":"run"}`},
		{"pr_merged", `{"artifactInputName":"prs","mergePolicy":"first"}`},
		{"pr_merged", `null`},
		{"pr_merged", `{} {}`},
	} {
		if err := p.ValidateParameters(tc.action, json.RawMessage(tc.params)); !errors.Is(err, problem.ErrInvalid) {
			t.Fatalf("accepted %s %s", tc.action, tc.params)
		}
	}
	result, err := p.HandleCallback(context.Background(), callbackInput(reviewPayload("approved", "old-commit", 71), "pull_request_review"))
	if err != nil {
		t.Fatal(err)
	}
	params := json.RawMessage(`{"artifactInputName":"prs","reviewStates":["approved"]}`)
	if matched, err := p.MatchSubjects("review_added", params, result.Events[0], nil); err != nil || matched.Matched {
		t.Fatalf("old review matched: %+v %v", matched, err)
	}
	params = json.RawMessage(`{"artifactInputName":"prs","reviewStates":["approved"],"requireCurrentHead":false}`)
	if matched, err := p.MatchSubjects("review_added", params, result.Events[0], nil); err != nil || !matched.Matched {
		t.Fatalf("explicit old review rejected: %+v %v", matched, err)
	}
}

func TestMergeAggregationRequiresNonemptyVerifiedSet(t *testing.T) {
	p := NewProvider()
	event := stageintegrationmodel.ProviderEvent{ActionID: "pr_merged", Subject: stageintegrationmodel.Subject{Merged: true}}
	params := json.RawMessage(`{"artifactInputName":"prs"}`)
	for _, subjects := range [][]stageintegrationmodel.Subject{nil, {{Merged: true}, {Merged: false}}} {
		decision, err := p.MatchSubjects("pr_merged", params, event, subjects)
		if err != nil || decision.Matched {
			t.Fatalf("premature aggregate: %+v %v", decision, err)
		}
	}
	decision, err := p.MatchSubjects("pr_merged", params, event, []stageintegrationmodel.Subject{{Merged: true}, {Merged: true}})
	if err != nil || !decision.Matched || !decision.OncePerObservation {
		t.Fatalf("aggregate=%+v %v", decision, err)
	}
	decision, err = p.MatchSubjects("pr_merged", json.RawMessage(`{"artifactInputName":"prs","mergePolicy":"any"}`), event, []stageintegrationmodel.Subject{{Merged: false}})
	if err != nil || !decision.Matched || decision.OncePerObservation {
		t.Fatalf("any=%+v %v", decision, err)
	}
}

func TestResolveAndReconcileUseKnownAPIAndPaginate(t *testing.T) {
	calls := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Errorf("missing auth: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/backend/pulls/42":
			_, _ = w.Write([]byte(prFixture(true)))
		case "/repos/acme/backend/pulls/42/reviews":
			if r.URL.Query().Get("page") == "2" {
				_, _ = fmt.Fprintf(w, "[%s]", reviewFixture("CHANGES_REQUESTED", "head-commit", 72))
				return
			}
			w.Header().Set("Link", `<`+server.URL+`/repos/acme/backend/pulls/42/reviews?page=2>; rel="next"`)
			_, _ = fmt.Fprintf(w, "[%s]", reviewFixture("APPROVED", "head-commit", 71))
		default:
			t.Errorf("unexpected API request: %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	p := GitHubPRIntegration{Client: server.Client(), APIBaseURL: server.URL}
	subject, err := p.ResolveSubject(context.Background(), json.RawMessage(`{}`), "fixture-token", "https://github.com/acme/backend/pull/42")
	if err != nil {
		t.Fatal(err)
	}
	if subject.RepositoryID != "123" || !subject.Merged {
		t.Fatalf("subject=%+v", subject)
	}
	result, err := p.Reconcile(context.Background(), json.RawMessage(`{}`), "fixture-token", "review_added", subject, time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 2 || calls != 4 {
		t.Fatalf("events=%d calls=%d", len(result.Events), calls)
	}
	prior := calls
	for _, raw := range []string{"https://evil.example/acme/backend/pull/42", "http://github.com/acme/backend/pull/42", "https://github.com/acme/backend/pull/42?token=secret", "https://github.com/acme/backend/pull/42#fragment", "https://user@github.com/acme/backend/pull/42"} {
		if _, err := p.ResolveSubject(context.Background(), json.RawMessage(`{}`), "fixture-token", raw); err == nil {
			t.Errorf("accepted URL %s", raw)
		}
	}
	if calls != prior {
		t.Fatal("invalid artifact URL fetched")
	}
}

func TestMergeReconciliationFetchesOneSnapshotWithoutReviews(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/repos/acme/backend/pulls/42" {
			t.Errorf("unexpected request: %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(prFixture(true)))
	}))
	defer server.Close()
	p := GitHubPRIntegration{Client: server.Client(), APIBaseURL: server.URL}
	// The returned subject must carry the live state, not the caller's stale head.
	subject := stageintegrationmodel.Subject{Kind: "code_review", ID: "50", RepositoryID: "123", URL: "https://github.com/acme/backend/pull/42", HeadSHA: "stale"}
	result, err := p.Reconcile(t.Context(), json.RawMessage(`{}`), "token", "pr_merged", subject, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !result.Subject.Merged || result.Subject.HeadSHA != "head-commit" || len(result.Events) != 1 {
		t.Fatalf("calls=%d result=%+v", calls, result)
	}
}

func TestValidateConnectionRequiresSelectedRepositoryAndPRAccess(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusForbidden, http.StatusNotFound} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/repositories/123":
					_, _ = w.Write([]byte(`{"id":123,"owner":{"login":"acme"},"name":"backend"}`))
				case "/repos/acme/backend/pulls":
					w.WriteHeader(status)
					_, _ = w.Write([]byte(`[]`))
				default:
					t.Errorf("unexpected validation request: %s", r.URL)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			p := GitHubPRIntegration{Client: server.Client(), APIBaseURL: server.URL}
			err := p.ValidateConnection(t.Context(), json.RawMessage(`{}`), "token", []string{"123"})
			if calls != 2 || (err == nil) != (status == http.StatusOK) {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}
