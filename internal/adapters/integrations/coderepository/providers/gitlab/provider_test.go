package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	coderepositorymodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
)

func TestGitLabProviderDiscoveryAndAuthentication(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != "secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/api/v4/user" {
			_, _ = w.Write([]byte(`{"id":1}`))
			return
		}
		if r.URL.Query().Get("page") == "1" {
			_, _ = w.Write([]byte(`[{"id":42,"path_with_namespace":"group/subgroup/project","http_url_to_repo":"` + server.URL + `/group/subgroup/project.git","visibility":"private","default_branch":"main"}]`))
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	provider := Provider{Name: ServerProviderName, Client: server.Client()}
	if err := provider.ValidateAuthentication(context.Background(), json.RawMessage(`{"baseUrl":"`+server.URL+`"}`), coderepositorymodel.Credentials{Token: "secret"}); err != nil {
		t.Fatal(err)
	}
	page, err := provider.DiscoverRepositories(context.Background(), json.RawMessage(`{"baseUrl":"`+server.URL+`"}`), coderepositorymodel.Credentials{Token: "secret"}, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Repositories) != 1 || page.Repositories[0].ID != "42" || page.Repositories[0].Name != "group/subgroup/project" || !page.Repositories[0].Private {
		t.Fatalf("repositories=%+v", page.Repositories)
	}
}

func TestGitLabConfigAndRepositoryURLValidation(t *testing.T) {
	cloud := NewGitLabCloudProvider()
	for _, cfg := range []string{`{}`} {
		if err := cloud.ValidateConfig(json.RawMessage(cfg)); err != nil {
			t.Fatalf("cloud config %s: %v", cfg, err)
		}
	}
	server := NewGitLabServerProvider()
	for _, cfg := range []string{`{}`, `{"baseUrl":"http://gitlab.example"}`, `{"baseUrl":"https://user@gitlab.example"}`} {
		if err := server.ValidateConfig(json.RawMessage(cfg)); err == nil {
			t.Errorf("accepted config %s", cfg)
		}
	}
	if err := server.ValidateConfig(json.RawMessage(`{"baseUrl":"https://gitlab.example/gitlab"}`)); err != nil {
		t.Fatal(err)
	}
	got, err := cloud.NormalizeRepositoryURL("https://gitlab.com/group/subgroup/project")
	if err != nil || got != "https://gitlab.com/group/subgroup/project.git" {
		t.Fatalf("url=%q err=%v", got, err)
	}
	for _, value := range []string{"http://gitlab.com/group/project", "https://gitlab.com/group/project/-/tree/main", "https://user@gitlab.com/group/project"} {
		if _, err = cloud.NormalizeRepositoryURL(value); err == nil {
			t.Errorf("accepted URL %q", value)
		}
	}
}

func TestGitLabServerRepositoryURLMustMatchConfiguredInstance(t *testing.T) {
	provider := NewGitLabServerProvider()
	if err := provider.validateCloneURL("https://gitlab.example/gitlab", "https://gitlab.example/gitlab/group/project.git"); err != nil {
		t.Fatal(err)
	}
	for _, cloneURL := range []string{
		"https://other.example/gitlab/group/project.git",
		"https://gitlab.example/group/project.git",
	} {
		if err := provider.validateCloneURL("https://gitlab.example/gitlab", cloneURL); err == nil {
			t.Errorf("accepted repository URL %q", cloneURL)
		}
	}
}

func TestSDKDiscoverySupportsSubpathsAndPaginationHeaders(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != "secret" {
			t.Error("missing credential")
		}
		if r.URL.Path == "/gitlab/api/v4/user" {
			_, _ = w.Write([]byte(`{"id":1}`))
			return
		}
		if r.URL.Path != "/gitlab/api/v4/projects" || r.URL.Query().Get("membership") != "true" ||
			r.URL.Query().Get("simple") != "true" || r.URL.Query().Get("order_by") != "name" || r.URL.Query().Get("sort") != "asc" ||
			r.URL.Query().Get("per_page") != "50" {
			t.Errorf("unexpected discovery request: %s", r.URL)
		}
		w.Header().Set("X-Next-Page", "7")
		if r.URL.Query().Get("page") == "7" {
			w.Header().Set("X-Next-Page", "")
		}
		_, _ = w.Write([]byte(`[{"id":42,"path_with_namespace":"group/project","http_url_to_repo":"` + server.URL + `/gitlab/group/project.git","visibility":"public","default_branch":"trunk"}]`))
	}))
	defer server.Close()
	provider := Provider{Name: ServerProviderName, Client: server.Client()}
	cfg := json.RawMessage(`{"baseUrl":"` + server.URL + `/gitlab"}`)
	credential := coderepositorymodel.Credentials{Token: "secret"}
	if err := provider.ValidateAuthentication(context.Background(), cfg, credential); err != nil {
		t.Fatal(err)
	}
	page, err := provider.DiscoverRepositories(context.Background(), cfg, credential, 0, 101)
	if err != nil || page.NextPage != 7 || len(page.Repositories) != 1 || page.Repositories[0].Private || page.Repositories[0].DefaultBranch != "trunk" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	last, err := provider.DiscoverRepositories(context.Background(), cfg, credential, page.NextPage, 50)
	if err != nil || last.NextPage != 0 {
		t.Fatalf("last page=%+v err=%v", last, err)
	}
}

func TestSDKFailuresAreSanitizedWithoutRetries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		authCode string
		listCode string
	}{
		{"unauthorized", http.StatusUnauthorized, `{"message":"secret-token"}`, "gitlab_authentication_failed", "gitlab_token_invalid_or_revoked"},
		{"forbidden", http.StatusForbidden, `{"message":"secret-token"}`, "gitlab_authentication_failed", "gitlab_permission_denied"},
		{"unavailable", http.StatusServiceUnavailable, `{"message":"secret-token"}`, "gitlab_connection_failed", "gitlab_repository_request_failed"},
		{"rate_limited", http.StatusTooManyRequests, `{"message":"secret-token"}`, "gitlab_connection_failed", "gitlab_repository_request_failed"},
		{"invalid_response", http.StatusOK, `{"secret-token":`, "gitlab_connection_failed", "gitlab_repository_response_invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			provider := Provider{Name: ServerProviderName, Client: server.Client()}
			cfg := json.RawMessage(`{"baseUrl":"` + server.URL + `"}`)
			credential := coderepositorymodel.Credentials{Token: "secret-token"}
			errs := []error{provider.ValidateAuthentication(context.Background(), cfg, credential)}
			_, err := provider.DiscoverRepositories(context.Background(), cfg, credential, 1, 50)
			errs = append(errs, err)
			for i, code := range []string{tc.authCode, tc.listCode} {
				var providerErr *coderepositorymodel.ProviderError
				if !errors.As(errs[i], &providerErr) || providerErr.Code != code || providerErr.Status != http.StatusBadGateway || strings.Contains(errs[i].Error(), "secret-token") {
					t.Fatalf("expected sanitized %s, got %v", code, errs[i])
				}
			}
			if requests.Load() != 2 {
				t.Fatalf("SDK retried failed requests: %d", requests.Load())
			}
		})
	}
}

func TestSDKDiscoveryRejectsForeignCloneURLsAndCancelledRequests(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`[{"id":1,"http_url_to_repo":"https://other.example/group/project.git"}]`))
	}))
	defer server.Close()
	provider := Provider{Name: ServerProviderName, Client: server.Client()}
	cfg := json.RawMessage(`{"baseUrl":"` + server.URL + `"}`)
	credential := coderepositorymodel.Credentials{Token: "secret-token"}
	_, err := provider.DiscoverRepositories(context.Background(), cfg, credential, 1, 50)
	var providerErr *coderepositorymodel.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != "gitlab_repository_response_invalid" {
		t.Fatalf("foreign origin error=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := provider.DiscoverRepositories(ctx, cfg, credential, 1, 50); err == nil {
		t.Fatal("cancelled discovery succeeded")
	}
	if requests.Load() != 1 {
		t.Fatalf("cancelled discovery reached provider: %d requests", requests.Load())
	}
}
