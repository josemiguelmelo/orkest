package gitprovider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
)

type recordingCLIAuthAdapter struct {
	providers []string
	seen      []runconfig.GitCredential
}

func (a *recordingCLIAuthAdapter) ProviderNames() []string { return a.providers }

func (a *recordingCLIAuthAdapter) ConfigureCLI(_ string, environment []string, credentials []runconfig.GitCredential) ([]string, error) {
	a.seen = credentials
	return append(environment, "CLI_ADAPTER="+a.providers[0]), nil
}

func TestConfigureProviderCLIsSelectsCredentialsByProvider(t *testing.T) {
	credentials := []runconfig.GitCredential{
		{ProviderName: "github", RepositoryURL: "https://github.com/group/project.git", Token: "github-secret"},
		{ProviderName: "gitlab_server", RepositoryURL: "https://gitlab.example/group/project.git", Token: "gitlab-secret"},
	}
	github := &recordingCLIAuthAdapter{providers: []string{"github"}}
	gitlab := &recordingCLIAuthAdapter{providers: []string{"gitlab_cloud", "gitlab_server"}}
	environment, err := configureCLIs(t.TempDir(), []string{"PATH=/bin"}, credentials, []CLIAuthAdapter{github, gitlab})
	if err != nil {
		t.Fatal(err)
	}
	if len(github.seen) != 1 || github.seen[0].ProviderName != "github" {
		t.Fatalf("GitHub CLI auth credentials=%+v", github.seen)
	}
	if len(gitlab.seen) != 1 || gitlab.seen[0].ProviderName != "gitlab_server" {
		t.Fatalf("GitLab CLI auth credentials=%+v", gitlab.seen)
	}
	if !strings.Contains(strings.Join(environment, "\n"), "CLI_ADAPTER=gitlab_cloud") {
		t.Fatalf("adapter environment was not applied: %v", environment)
	}
}

func TestProviderCLIAdaptersScopeSecretsToMatchingCheckout(t *testing.T) {
	binDir := t.TempDir()
	for _, executable := range []string{"gh", "glab"} {
		if err := os.WriteFile(filepath.Join(binDir, executable), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binDir)
	runtimeRoot := t.TempDir()
	credentials := []runconfig.GitCredential{
		{ProviderName: "github", RepositoryURL: "https://github.com/acme/project.git", Token: "github-secret"},
		{ProviderName: "gitlab_server", ProviderConfig: []byte(`{"baseUrl":"https://gitlab.example/gitlab"}`), RepositoryURL: "https://gitlab.example/gitlab/group/project.git", Token: "gitlab-secret"},
	}
	environment, err := configureCLIs(runtimeRoot, []string{"PATH=" + binDir, "GH_TOKEN=stale", "GITLAB_TOKEN=stale"}, credentials, DefaultCLIAuthAdapters())
	if err != nil {
		t.Fatal(err)
	}
	values := envMap(environment)
	if values["ORKEST_GH_TOKEN_0"] != "github-secret" || values["ORKEST_GITLAB_TOKEN_0"] != "gitlab-secret" {
		t.Fatalf("provider token environment missing")
	}
	if values["GH_TOKEN"] != "" || values["GITLAB_TOKEN"] != "" {
		t.Fatalf("unscoped provider tokens leaked into runtime environment")
	}
	githubScript, err := os.ReadFile(filepath.Join(runtimeRoot, "github-cli", "gh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(githubScript), "https://github.com/acme/project") || strings.Contains(string(githubScript), "gitlab.example") || strings.Contains(string(githubScript), "github-secret") {
		t.Fatalf("GitHub CLI wrapper has incorrect repository scope or embeds a token: %s", githubScript)
	}
	gitlabScript, err := os.ReadFile(filepath.Join(runtimeRoot, "gitlab-cli", "glab"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"https://gitlab.example/gitlab/group/project", "GITLAB_HOST='gitlab.example'", "GITLAB_SUBFOLDER='gitlab'", "GITLAB_TOKEN=\"${ORKEST_GITLAB_TOKEN_0:?GitLab credential unavailable}\""} {
		if !strings.Contains(string(gitlabScript), expected) {
			t.Errorf("GitLab CLI wrapper missing %q", expected)
		}
	}
	if strings.Contains(string(gitlabScript), "gitlab-secret") || strings.Contains(string(gitlabScript), "github.com") {
		t.Fatalf("GitLab CLI wrapper contains an unrelated credential: %s", gitlabScript)
	}
}

func envMap(environment []string) map[string]string {
	values := make(map[string]string, len(environment))
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			values[name] = value
		}
	}
	return values
}
