package github

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
	if err := provider.Clone(ctx, bare, checkout); err != nil {
		t.Fatalf("clone: %v", err)
	}
	writeTestFile(t, filepath.Join(work, "state.txt"), "second")
	runTestGit(t, work, "commit", "-am", "second")
	runTestGit(t, work, "push", "origin", "main")
	writeTestFile(t, filepath.Join(checkout, "state.txt"), "local dirty state")
	if err := provider.Sync(ctx, bare, checkout); err != nil {
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
	if err := NewGitHubProvider().Sync(context.Background(), second, checkout); err == nil {
		t.Fatal("Sync succeeded for a checkout with a different origin")
	}
}

func TestPrepareTaskCheckoutCreatesAndReusesTaskBranch(t *testing.T) {
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
	if err := provider.PrepareTaskCheckout(context.Background(), remote, checkout, branch); err != nil {
		t.Fatalf("prepare checkout: %v", err)
	}
	if got := gitOutputTest(t, checkout, "branch", "--show-current"); got != branch {
		t.Fatalf("branch = %q, want %q", got, branch)
	}
	writeTestFile(t, filepath.Join(checkout, "work.txt"), "preserve me")
	if err := provider.PrepareTaskCheckout(context.Background(), remote, checkout, branch); err != nil {
		t.Fatalf("reuse checkout: %v", err)
	}
	if got := readTestFile(t, filepath.Join(checkout, "work.txt")); got != "preserve me" {
		t.Fatalf("working tree was not preserved: %q", got)
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
