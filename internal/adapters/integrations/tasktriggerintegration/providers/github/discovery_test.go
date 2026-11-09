package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	codemodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
	"github.com/josemiguelmelo/orkest/internal/server/pullrequestlink"
)

type credentialsFake struct{ t *testing.T }

func (f credentialsFake) CodingCredential(_ context.Context, workspace, integration string) (codemodel.Credentials, error) {
	f.t.Helper()
	if workspace != "workspace" || integration != "coding" {
		f.t.Fatal("credential scope mismatch")
	}
	return codemodel.Credentials{Token: "secret-fixture"}, nil
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDiscoveryRecoversReviewsWithWebhookIdentities(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second).Add(123456789 * time.Nanosecond)
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer secret-fixture" {
			t.Fatal("missing scoped authentication")
		}
		var body string
		switch r.URL.Path {
		case "/repositories/123":
			body = `{"id":123,"name":"repo","owner":{"login":"owner"}}`
		case "/repos/owner/repo/pulls":
			body = fmt.Sprintf(`[{"number":4,"updated_at":%q,"merged_at":%q,"html_url":"https://github.com/owner/repo/pull/4","head":{"ref":"user/skill","sha":"abc","repo":{"id":123}}}]`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
		case "/repos/owner/repo/pulls/4/reviews":
			body = fmt.Sprintf(`[{"id":9,"state":"CHANGES_REQUESTED","submitted_at":%q},{"id":10,"state":"APPROVED","submitted_at":%q},{"id":11,"state":"PENDING"}]`, now.Format(time.RFC3339), now.Add(-time.Hour).Format(time.RFC3339))
		default:
			t.Fatalf("unexpected API request: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	discovery := Discovery{Credentials: credentialsFake{t}, Client: client}
	repositories := []pullrequestlink.DiscoveryRepository{{ProviderRepositoryID: "123", IntegrationID: "coding", Candidates: []pullrequestlink.Candidate{{EligibleForLinking: true, Branch: "user/skill", CommitID: "abc"}}}}
	events, err := discovery.Discover(context.Background(), "workspace", json.RawMessage(`{"repositoryIds":["123"]}`), now.Add(-time.Minute), repositories)
	if err != nil || len(events) != 3 {
		t.Fatalf("events: %+v %v", events, err)
	}
	if events[1].Key != "merged:123:4" || events[2].Key != "review:9" || events[2].HeadCommitID != "abc" {
		t.Fatalf("wrong semantic identities: %+v", events)
	}
	for _, tc := range []struct {
		name, kind, body, wantKey string
		index                     int
	}{
		{name: "discovered", kind: "pull_request", body: fmt.Sprintf(`{"action":"synchronize","repository":{"id":123,"full_name":"owner/repo"},"pull_request":{"number":4,"updated_at":%q,"head":{"sha":"abc"}}}`, now.Format(time.RFC3339Nano)), wantKey: "discovered:123:4:abc:" + now.Format(time.RFC3339Nano), index: 0},
		{name: "merged", kind: "pull_request", body: fmt.Sprintf(`{"action":"closed","repository":{"id":123,"full_name":"owner/repo"},"pull_request":{"number":4,"merged":true,"merged_at":%q}}`, now.Format(time.RFC3339Nano)), wantKey: "merged:123:4", index: 1},
		{name: "review", kind: "pull_request_review", body: fmt.Sprintf(`{"action":"submitted","repository":{"id":123,"full_name":"owner/repo"},"pull_request":{"number":4},"review":{"id":9,"submitted_at":%q}}`, now.Format(time.RFC3339)), wantKey: "review:9", index: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, webhookEvents, err := NewGitHubProvider().VerifyAndDecode(context.Background(), signedRequest(tc.kind, tc.body), json.RawMessage(`{"repositoryIds":["123"]}`), "test-webhook-secret")
			if err != nil || len(webhookEvents) != 1 {
				t.Fatalf("webhook events: %+v %v", webhookEvents, err)
			}
			if webhookEvents[0].Key != tc.wantKey || events[tc.index].Key != tc.wantKey || webhookEvents[0].Type != events[tc.index].Type {
				t.Fatalf("event identity mismatch: webhook=%+v discovery=%+v, want key %q", webhookEvents[0], events[tc.index], tc.wantKey)
			}
		})
	}
	events, err = discovery.Discover(context.Background(), "workspace", json.RawMessage(`{"repositoryIds":["123"]}`), now.Add(time.Minute), repositories)
	if err != nil || len(events) != 0 {
		t.Fatalf("settings save replayed historical events: %+v %v", events, err)
	}
}

func TestDiscoveryVisitsConfiguredRepositories(t *testing.T) {
	visited := map[string]bool{}
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		var body string
		switch r.URL.Path {
		case "/repositories/123":
			body = `{"id":123,"name":"one","owner":{"login":"owner"}}`
		case "/repositories/456":
			body = `{"id":456,"name":"two","owner":{"login":"owner"}}`
		case "/repos/owner/one/pulls", "/repos/owner/two/pulls":
			body = `[]`
		default:
			t.Fatalf("unconfigured repository requested: %s", r.URL.Path)
		}
		visited[r.URL.Path] = true
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	discovery := Discovery{Credentials: credentialsFake{t}, Client: client}
	repositories := []pullrequestlink.DiscoveryRepository{{ProviderRepositoryID: "123", IntegrationID: "coding"}, {ProviderRepositoryID: "456", IntegrationID: "coding"}, {ProviderRepositoryID: "999", IntegrationID: "coding"}}
	if _, err := discovery.Discover(context.Background(), "workspace", json.RawMessage(`{"repositoryIds":["123","456"]}`), time.Now(), repositories); err != nil {
		t.Fatal(err)
	}
	if !visited["/repositories/123"] || !visited["/repositories/456"] {
		t.Fatalf("missing repository: %v", visited)
	}
}
