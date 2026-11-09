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
	now := time.Now().UTC().Truncate(time.Second)
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer secret-fixture" {
			t.Fatal("missing scoped authentication")
		}
		var body string
		switch r.URL.Path {
		case "/repositories/123":
			body = `{"id":123,"name":"repo","owner":{"login":"owner"}}`
		case "/repos/owner/repo/pulls":
			body = fmt.Sprintf(`[{"number":4,"updated_at":%q,"merged_at":%q,"html_url":"https://github.com/owner/repo/pull/4","head":{"ref":"user/skill","sha":"abc","repo":{"id":123}}}]`, now.Format(time.RFC3339), now.Format(time.RFC3339))
		case "/repos/owner/repo/pulls/4/reviews":
			body = fmt.Sprintf(`[{"id":9,"state":"CHANGES_REQUESTED","submitted_at":%q},{"id":10,"state":"APPROVED","submitted_at":%q},{"id":11,"state":"PENDING"}]`, now.Format(time.RFC3339), now.Add(-time.Hour).Format(time.RFC3339))
		default:
			t.Fatalf("unexpected API request: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	discovery := Discovery{Credentials: credentialsFake{t}, Client: client}
	repositories := []pullrequestlink.DiscoveryRepository{{ProviderRepositoryID: "123", IntegrationID: "coding", Candidates: []pullrequestlink.Candidate{{Branch: "user/skill", CommitID: "abc"}}}}
	events, err := discovery.Discover(context.Background(), "workspace", json.RawMessage(`{"repositoryId":"123"}`), now.Add(-time.Minute), repositories)
	if err != nil || len(events) != 3 {
		t.Fatalf("events: %+v %v", events, err)
	}
	if events[1].Key != "merged:123:4" || events[2].Key != "review:9" || events[2].HeadCommitID != "abc" {
		t.Fatalf("wrong semantic identities: %+v", events)
	}
	events, err = discovery.Discover(context.Background(), "workspace", json.RawMessage(`{"repositoryId":"123"}`), now.Add(time.Minute), repositories)
	if err != nil || len(events) != 0 {
		t.Fatalf("settings save replayed historical events: %+v %v", events, err)
	}
}
