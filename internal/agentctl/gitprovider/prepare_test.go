package gitprovider

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
)

func TestPrepareRuntimeConfiguresGitForMultipleRepositories(t *testing.T) {
	workDir := t.TempDir()
	credentials := []runconfig.GitCredential{
		{ProviderName: "github", RepositoryURL: "https://github.com/acme/one.git", RepositoryPath: "repositories/github/one", Token: "github-secret"},
		{ProviderName: "gitlab_server", RepositoryURL: "https://gitlab.example/group/two.git", RepositoryPath: "repositories/gitlab/two", Token: "gitlab-secret"},
	}
	for _, credential := range credentials {
		path := filepath.Join(workDir, credential.RepositoryPath)
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		command := exec.Command("git", "-C", path, "init", "--quiet")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("initialize test repository: %s: %v", output, err)
		}
	}

	environment, err := PrepareRuntime(context.Background(), RuntimePreparation{
		RuntimeRoot: t.TempDir(),
		WorkDir:     workDir,
		Environment: []string{"PATH=/bin", "KEEP=value", "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=stale", "GIT_CONFIG_VALUE_0=stale"},
		Credentials: credentials,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	values := environmentMap(environment)
	if values["GIT_CONFIG_COUNT"] != "2" || values["GIT_CONFIG_KEY_0"] != "http.https://github.com/acme/one.git.extraheader" || values["GIT_CONFIG_KEY_1"] != "http.https://gitlab.example/group/two.git.extraheader" {
		t.Fatalf("Git credentials are not scoped to each repository: %v", values)
	}
	if values["GIT_CONFIG_VALUE_0"] == "stale" || values["GIT_CONFIG_VALUE_1"] == "stale" || values["KEEP"] != "value" || values["PATH"] != "/bin" {
		t.Fatalf("environment was not replaced and preserved correctly: %v", values)
	}
	for _, credential := range credentials {
		path := filepath.Join(workDir, credential.RepositoryPath)
		for key, expected := range map[string]string{"user.name": "Orkest Agent", "user.email": "agent@orkest.local"} {
			output, err := exec.Command("git", "-C", path, "config", "--local", "--get", key).Output()
			if err != nil || string(output) != expected+"\n" {
				t.Errorf("repository %s has %s=%q, want %q (error: %v)", credential.RepositoryPath, key, output, expected, err)
			}
		}
	}
}
