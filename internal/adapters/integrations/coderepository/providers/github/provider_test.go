package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	coderepositorymodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
)

func TestAuthenticatedRepositoryDiscoveryPaginatesAndSanitizesFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "4" {
			http.Error(w, "rate limit response contains secret-token", http.StatusForbidden)
			return
		}
		if r.URL.Query().Get("page") == "3" {
			http.Error(w, "token secret-token rejected", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			http.Error(w, "credential rejected secret-token", http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("page") == "1" {
			_, _ = w.Write([]byte(`[{"id":1,"full_name":"acme/one","clone_url":"https://github.com/acme/one.git","private":true,"default_branch":"trunk"}]`))
			return
		}
		_, _ = w.Write([]byte(`[{"id":2,"full_name":"acme/two","clone_url":"https://github.com/acme/two.git","private":false,"default_branch":"release"}]`))
	}))
	defer server.Close()
	provider := GitHubProvider{Client: server.Client(), APIBaseURL: server.URL}
	first, err := provider.DiscoverRepositories(context.Background(), json.RawMessage(`{}`), coderepositorymodel.Credentials{Token: "secret-token"}, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Repositories) != 1 || first.NextPage != 2 || first.Repositories[0].DefaultBranch != "trunk" || !first.Repositories[0].Private {
		t.Fatalf("first page=%+v", first)
	}
	second, err := provider.DiscoverRepositories(context.Background(), json.RawMessage(`{}`), coderepositorymodel.Credentials{Token: "secret-token"}, first.NextPage, 1)
	if err != nil {
		t.Fatal(err)
	}
	if second.Repositories[0].ID != "2" || second.Repositories[0].DefaultBranch != "release" {
		t.Fatalf("second page=%+v", second)
	}
	_, err = provider.DiscoverRepositories(context.Background(), json.RawMessage(`{}`), coderepositorymodel.Credentials{Token: "secret-token"}, 3, 10)
	var providerErr *coderepositorymodel.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != "github_token_invalid_or_revoked" || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("sanitized auth error=%v", err)
	}
	_, err = provider.DiscoverRepositories(context.Background(), json.RawMessage(`{}`), coderepositorymodel.Credentials{Token: "secret-token"}, 4, 10)
	if !errors.As(err, &providerErr) || providerErr.Code != "github_permission_denied_or_rate_limited" || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("sanitized rate-limit error=%v", err)
	}
}

func TestAuthenticatedGitOperationsKeepTokenOutOfArguments(t *testing.T) {
	bin := t.TempDir()
	logPath := filepath.Join(bin, "git-args.log")
	gitPath := filepath.Join(bin, "git")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + logPath + "'\ncase \"$*\" in *'remote get-url origin'*) echo 'https://github.com/acme/project.git';; esac\n"
	if err := os.WriteFile(gitPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	path := filepath.Join(t.TempDir(), "checkout")
	if err := os.MkdirAll(filepath.Join(path, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	token := "do-not-leak-this-token"
	provider := NewGitHubProvider()
	err := provider.SyncAuthenticated(context.Background(), coderepositorymodel.RepositoryMetadata{ID: "42", CloneURL: "https://github.com/acme/project.git", DefaultBranch: "release"}, coderepositorymodel.Credentials{Token: token}, path)
	if err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(args), token) {
		t.Fatal("token appeared in Git process arguments")
	}
	if !strings.Contains(string(args), "fetch origin release") {
		t.Fatalf("default branch fetch missing from git calls: %s", args)
	}
}

func TestAuthenticatedTaskCheckoutRejectsMismatchedOriginBeforeUsingCredentials(t *testing.T) {
	bin := t.TempDir()
	logPath := filepath.Join(bin, "git-args.log")
	gitPath := filepath.Join(bin, "git")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + logPath + "'\necho 'https://github.com/other/repository.git'\n"
	if err := os.WriteFile(gitPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	checkout := filepath.Join(t.TempDir(), "checkout")
	if err := os.MkdirAll(filepath.Join(checkout, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	err := NewGitHubProvider().PrepareAuthenticatedTaskCheckout(context.Background(), coderepositorymodel.RepositoryMetadata{ID: "42", CloneURL: "https://github.com/acme/project.git", DefaultBranch: "main"}, coderepositorymodel.Credentials{Token: "sensitive-token"}, checkout, "orkest/task-1")
	if err == nil || !strings.Contains(err.Error(), "origin") || strings.Contains(err.Error(), "sensitive-token") {
		t.Fatalf("origin mismatch error=%v", err)
	}
	args, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(args), "-c credential.helper") || strings.Contains(string(args), "sensitive-token") {
		t.Fatalf("credential operation occurred before origin validation: %s", args)
	}
}

func TestCloneAndSync(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable is unavailable")
	}
	ctx := context.Background()
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	work := filepath.Join(root, "source")
	checkout := filepath.Join(root, "checkout")
	runTestGit(t, root, "init", "--bare", "--initial-branch=main", bare)
	runTestGit(t, root, "clone", bare, work)
	runTestGit(t, work, "config", "user.email", "test@example.com")
	runTestGit(t, work, "config", "user.name", "Test")
	writeTestFile(t, filepath.Join(work, "state.txt"), "first")
	runTestGit(t, work, "add", "state.txt")
	runTestGit(t, work, "commit", "-m", "first")
	runTestGit(t, work, "push", "origin", "main")
	provider := NewGitHubProvider()
	if err := provider.ClonePublic(ctx, bare, checkout); err != nil {
		t.Fatalf("clone: %v", err)
	}
	writeTestFile(t, filepath.Join(work, "state.txt"), "second")
	runTestGit(t, work, "commit", "-am", "second")
	runTestGit(t, work, "push", "origin", "main")
	writeTestFile(t, filepath.Join(checkout, "state.txt"), "local dirty state")
	if err := provider.SyncPublic(ctx, bare, checkout); err != nil {
		t.Fatalf("sync main: %v", err)
	}
	if got := readTestFile(t, filepath.Join(checkout, "state.txt")); got != "second" {
		t.Fatalf("checkout state after sync = %q, want latest remote main", got)
	}
}

func TestSyncRejectsDifferentOrigin(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.git")
	second := filepath.Join(root, "second.git")
	checkout := filepath.Join(root, "checkout")
	runTestGit(t, root, "init", "--bare", "--initial-branch=main", first)
	runTestGit(t, root, "init", "--bare", "--initial-branch=main", second)
	runTestGit(t, root, "clone", first, checkout)
	if err := NewGitHubProvider().SyncPublic(context.Background(), second, checkout); err == nil {
		t.Fatal("Sync succeeded for a checkout with a different origin")
	}
}

func TestPreparePublicTaskCheckoutCreatesAndReusesTaskBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable is unavailable")
	}
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	source := filepath.Join(root, "source")
	checkout := filepath.Join(root, "task", "repo")
	runTestGit(t, root, "init", "--bare", "--initial-branch=main", remote)
	runTestGit(t, root, "clone", remote, source)
	runTestGit(t, source, "config", "user.email", "test@example.com")
	runTestGit(t, source, "config", "user.name", "Test")
	writeTestFile(t, filepath.Join(source, "README.md"), "task source")
	runTestGit(t, source, "add", "README.md")
	runTestGit(t, source, "commit", "-m", "initial")
	runTestGit(t, source, "push", "origin", "main")

	provider := NewGitHubProvider()
	branch := "orkest/task-task-id"
	if err := provider.PreparePublicTaskCheckout(context.Background(), remote, checkout, branch); err != nil {
		t.Fatalf("prepare checkout: %v", err)
	}
	if got := gitOutputTest(t, checkout, "branch", "--show-current"); got != branch {
		t.Fatalf("branch = %q, want %q", got, branch)
	}
	writeTestFile(t, filepath.Join(checkout, "work.txt"), "preserve me")
	if err := provider.PreparePublicTaskCheckout(context.Background(), remote, checkout, branch); err != nil {
		t.Fatalf("reuse checkout: %v", err)
	}
	if got := readTestFile(t, filepath.Join(checkout, "work.txt")); got != "preserve me" {
		t.Fatalf("working tree was not preserved: %q", got)
	}
}

func TestNormalizeRepositoryURLRequiresCanonicalGitHubRepository(t *testing.T) {
	provider := NewGitHubProvider()
	got, err := provider.NormalizeRepositoryURL(" https://github.com/acme/project/ ")
	if err != nil || got != "https://github.com/acme/project.git" {
		t.Fatalf("normalized url=%q err=%v", got, err)
	}
	for _, value := range []string{"http://github.com/acme/project", "https://github.com/acme/project/tree/main", "https://user@github.com/acme/project", "https://example.com/acme/project"} {
		if _, err = provider.NormalizeRepositoryURL(value); err == nil {
			t.Errorf("accepted invalid URL %q", value)
		}
	}
}

func runTestGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func gitOutputTest(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(output))
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}
