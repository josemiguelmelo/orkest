package gitprovider

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
)

func TestPrepareRuntimeCommitIdentity(t *testing.T) {
	tests := []struct {
		name        string
		environment []string
		identity    string
	}{
		{name: "defaults", environment: []string{"ORKEST_GIT_AUTHOR_NAME=Orkest Agent", "ORKEST_GIT_AUTHOR_EMAIL=agent@orkest.local"}, identity: "Orkest Agent <agent@orkest.local>"},
		{name: "custom", environment: []string{"ORKEST_GIT_AUTHOR_NAME=Build Agent", "ORKEST_GIT_AUTHOR_EMAIL=build@example.com"}, identity: "Build Agent <build@example.com>"},
	}
	for _, test := range tests {
		for _, authenticated := range []bool{false, true} {
			name := test.name + "/unauthenticated"
			var credentials []configmodel.GitCredential
			if authenticated {
				name = test.name + "/authenticated"
				credentials = []configmodel.GitCredential{{ProviderName: "github", RepositoryURL: "https://github.com/acme/", Token: "test-token"}}
			}
			t.Run(name, func(t *testing.T) {
				workDir := t.TempDir()
				base := []string{
					"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(),
					"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull,
					"GIT_AUTHOR_NAME=Stale", "GIT_AUTHOR_EMAIL=stale@example.com",
					"GIT_COMMITTER_NAME=Stale", "GIT_COMMITTER_EMAIL=stale@example.com",
				}
				environment, err := PrepareRuntime(context.Background(), RuntimePreparation{
					RuntimeRoot: t.TempDir(), WorkDir: workDir,
					Environment: append(base, test.environment...), Credentials: credentials,
				}, nil)
				if err != nil {
					t.Fatal(err)
				}
				git := func(args ...string) string {
					t.Helper()
					command := exec.Command("git", args...)
					command.Dir, command.Env = workDir, environment
					output, err := command.CombinedOutput()
					if err != nil {
						t.Fatalf("git %v: %s: %v", args, output, err)
					}
					return strings.TrimSpace(string(output))
				}
				git("init", "--quiet")
				git("-c", "commit.gpgsign=false", "-c", "core.hooksPath="+os.DevNull, "commit", "--allow-empty", "--quiet", "-m", "identity test")
				want := test.identity + "\n" + test.identity
				if got := git("show", "-s", "--format=%an <%ae>%n%cn <%ce>", "HEAD"); got != want {
					t.Fatalf("commit author/committer = %q, want %q", got, want)
				}
			})
		}
	}
}

func TestPrepareRuntimeConfiguresGitForMultipleRepositories(t *testing.T) {
	workDir := t.TempDir()
	credentials := []configmodel.GitCredential{
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
		Environment: []string{"PATH=/bin", "KEEP=value", "ORKEST_GIT_AUTHOR_NAME=Orkest Agent", "ORKEST_GIT_AUTHOR_EMAIL=agent@orkest.local", "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=stale", "GIT_CONFIG_VALUE_0=stale"},
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
	for name, expected := range map[string]string{"GIT_AUTHOR_NAME": "Orkest Agent", "GIT_COMMITTER_NAME": "Orkest Agent", "GIT_AUTHOR_EMAIL": "agent@orkest.local", "GIT_COMMITTER_EMAIL": "agent@orkest.local"} {
		if values[name] != expected {
			t.Errorf("incorrect Git identity variable %s", name)
		}
	}
}
