package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/server/coderepository"
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
	if err := provider.ValidateAuthentication(context.Background(), json.RawMessage(`{"baseUrl":"`+server.URL+`"}`), coderepository.Credentials{Token: "secret"}); err != nil {
		t.Fatal(err)
	}
	page, err := provider.DiscoverRepositories(context.Background(), json.RawMessage(`{"baseUrl":"`+server.URL+`"}`), coderepository.Credentials{Token: "secret"}, 1, 1)
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
