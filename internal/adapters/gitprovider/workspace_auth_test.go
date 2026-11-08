package gitprovider

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	"github.com/josemiguelmelo/orkest/internal/contracts/configvalidation"
)

func TestWorkspaceAuthWithoutLinkedCheckouts(t *testing.T) {
	binDir := t.TempDir()
	for _, name := range []string{"gh", "glab"} {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte("#!/bin/sh\nprintf '%s' \"${GH_TOKEN:-}${GITLAB_TOKEN:-}\"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	credentials := []configmodel.GitCredential{
		{ProviderName: "github", RepositoryURL: "https://github.com/", Token: "test-github"},
		{ProviderName: "gitlab_server", ProviderConfig: []byte(`{"baseUrl":"https://gitlab.example/gitlab"}`), RepositoryURL: "https://gitlab.example/gitlab/", Token: "test-gitlab"},
	}
	for _, credential := range credentials {
		if issue := configvalidation.GitCredential(credential.ProviderName, credential.Token, credential.RepositoryPath, credential.RepositoryURL, credential.ProviderConfig); issue != nil {
			t.Fatalf("workspace credential rejected: %v", issue)
		}
	}
	runtimeRoot := t.TempDir()
	checkout := t.TempDir()
	environment, err := PrepareRuntime(context.Background(), RuntimePreparation{RuntimeRoot: runtimeRoot, WorkDir: checkout, Environment: os.Environ(), Credentials: credentials}, DefaultCLIAuthAdapters())
	if err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "-C", checkout, "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	if output, err := exec.Command("git", "-C", checkout, "remote", "add", "origin", "https://github.com/acme/unlinked.git").CombinedOutput(); err != nil {
		t.Fatalf("git remote: %v: %s", err, output)
	}
	for _, tc := range []struct{ remote, directory, cli, token string }{
		{"https://github.com/acme/unlinked.git", "github-cli", "gh", "test-github"},
		{"https://github.com.evil.example/acme/project.git", "github-cli", "gh", ""},
		{"https://gitlab.example/gitlab/group/project.git", "gitlab-cli", "glab", "test-gitlab"},
		{"https://gitlab.example/other/project.git", "gitlab-cli", "glab", ""},
	} {
		cmd := exec.Command("git", "config", "--get-urlmatch", "http.extraheader", tc.remote)
		cmd.Env = environment
		output, err := cmd.Output()
		if (err == nil && len(output) > 0) != (tc.token != "") {
			t.Errorf("incorrect Git authentication scope for %s", tc.remote)
		}
		if output, err := exec.Command("git", "-C", checkout, "remote", "set-url", "origin", tc.remote).CombinedOutput(); err != nil {
			t.Fatalf("set origin: %v: %s", err, output)
		}
		cmd = exec.Command(filepath.Join(runtimeRoot, tc.directory, tc.cli), "auth", "status")
		cmd.Dir, cmd.Env = checkout, environment
		output, err = cmd.Output()
		if tc.token == "" {
			if err == nil || len(output) != 0 {
				t.Errorf("CLI accepted unrelated origin %s", tc.remote)
			}
		} else if err != nil || string(output) != tc.token {
			t.Errorf("CLI did not authenticate %s: %v", tc.remote, err)
		}
	}
}
