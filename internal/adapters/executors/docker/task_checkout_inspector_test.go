package docker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
)

func TestTaskCheckoutInspectorFindsSelectedGitCheckoutWithoutMutation(t *testing.T) {
	dataRoot := t.TempDir()
	workDir, stateDir := taskstorage.Directories(dataRoot, "org", "workspace", "task")
	checkout := filepath.Join(workDir, "repositories", "github", "registration")
	initInspectorCheckout(t, checkout)
	tracked := filepath.Join(checkout, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("before\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	runGit(t, checkout, "add", "tracked.txt")
	runGit(t, checkout, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "fixture")
	if err := os.WriteFile(filepath.Join(checkout, "untracked.txt"), []byte("untracked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(stateDir, "session-marker")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateFile, []byte("session-state\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeBranch := runGit(t, checkout, "branch", "--show-current")
	beforeStatus := runGit(t, checkout, "status", "--porcelain")
	beforeMode := fileMode(t, tracked)
	beforeUntrackedMode := fileMode(t, filepath.Join(checkout, "untracked.txt"))
	inspector := TaskCheckoutInspector{DataRoot: dataRoot}
	got, err := inspector.Inspect(context.Background(), "org", "workspace", "task", []taskeditor.Checkout{{RegistrationID: "registration", ProviderName: "github", Name: "service"}, {RegistrationID: "not-selected-existing", ProviderName: "github", Name: "missing"}})
	if err != nil {
		t.Fatal(err)
	}
	canonicalCheckout, err := filepath.EvalSymlinks(checkout)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Roots) != 1 || got.Roots[0].Source != canonicalCheckout || got.Roots[0].Path != "/workspace/repositories/github/registration" {
		t.Fatalf("unexpected roots: %#v", got.Roots)
	}
	if len(got.Missing) != 1 || got.Missing[0].RegistrationID != "not-selected-existing" {
		t.Fatalf("unexpected missing checkouts: %#v", got.Missing)
	}
	if runGit(t, checkout, "branch", "--show-current") != beforeBranch || runGit(t, checkout, "status", "--porcelain") != beforeStatus {
		t.Fatal("inspection changed Git state")
	}
	if fileMode(t, tracked) != beforeMode {
		t.Fatal("inspection changed file permissions")
	}
	if fileMode(t, filepath.Join(checkout, "untracked.txt")) != beforeUntrackedMode {
		t.Fatal("inspection changed untracked file permissions")
	}
	if data, err := os.ReadFile(filepath.Join(checkout, "tracked.txt")); err != nil || string(data) != "before\n" {
		t.Fatalf("tracked content changed: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(checkout, "untracked.txt")); err != nil || string(data) != "untracked\n" {
		t.Fatalf("untracked content changed: %v", err)
	}
	if data, err := os.ReadFile(stateFile); err != nil || string(data) != "session-state\n" {
		t.Fatalf("agent state changed: %v", err)
	}
}

func TestTaskCheckoutInspectorMissingCheckoutDoesNotCreateStorage(t *testing.T) {
	dataRoot := t.TempDir()
	inspector := TaskCheckoutInspector{DataRoot: dataRoot}
	got, err := inspector.Inspect(context.Background(), "org", "workspace", "task", []taskeditor.Checkout{{RegistrationID: "missing", ProviderName: "github", Name: "repo"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Missing) != 1 || got.Missing[0].RegistrationID != "missing" {
		t.Fatalf("unexpected missing result: %#v", got.Missing)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "tasks")); !os.IsNotExist(err) {
		t.Fatalf("inspector created task storage: %v", err)
	}
}

func TestTaskCheckoutInspectorRejectsSymlinkStoragePaths(t *testing.T) {
	dataRoot := t.TempDir()
	workDir, _ := taskstorage.Directories(dataRoot, "org", "workspace", "task")
	if err := os.MkdirAll(filepath.Dir(workDir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), workDir); err != nil {
		t.Fatal(err)
	}
	inspector := TaskCheckoutInspector{DataRoot: dataRoot}
	if _, err := inspector.Inspect(context.Background(), "org", "workspace", "task", []taskeditor.Checkout{{RegistrationID: "r", ProviderName: "github", Name: "repo"}}); err == nil {
		t.Fatal("expected task work symlink rejection")
	}
}

func TestTaskCheckoutInspectorRejectsCheckoutAndDanglingSymlinks(t *testing.T) {
	for _, targetExists := range []bool{true, false} {
		t.Run(map[bool]string{true: "checkout", false: "dangling"}[targetExists], func(t *testing.T) {
			dataRoot := t.TempDir()
			workDir, _ := taskstorage.Directories(dataRoot, "org", "workspace", "task")
			parent := filepath.Join(workDir, "repositories", "github")
			if err := os.MkdirAll(parent, 0o700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(t.TempDir(), "external-checkout")
			if targetExists {
				initInspectorCheckout(t, target)
			}
			if err := os.Symlink(target, filepath.Join(parent, "repo")); err != nil {
				t.Fatal(err)
			}
			inspector := TaskCheckoutInspector{DataRoot: dataRoot}
			if _, err := inspector.Inspect(context.Background(), "org", "workspace", "task", []taskeditor.Checkout{{RegistrationID: "repo", ProviderName: "github", Name: "repo"}}); err == nil {
				t.Fatal("expected checkout symlink rejection")
			}
		})
	}
}

func TestTaskCheckoutInspectorRejectsSymlinkParentForMissingRepository(t *testing.T) {
	dataRoot := t.TempDir()
	workDir, _ := taskstorage.Directories(dataRoot, "org", "workspace", "task")
	if err := os.MkdirAll(filepath.Join(workDir, "repositories"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(workDir, "repositories", "github")); err != nil {
		t.Fatal(err)
	}
	inspector := TaskCheckoutInspector{DataRoot: dataRoot}
	if _, err := inspector.Inspect(context.Background(), "org", "workspace", "task", []taskeditor.Checkout{{RegistrationID: "missing", ProviderName: "github", Name: "repo"}}); err == nil {
		t.Fatal("expected symlink parent rejection")
	}
}

func TestTaskCheckoutInspectorRejectsExternalGitAlternates(t *testing.T) {
	dataRoot := t.TempDir()
	workDir, _ := taskstorage.Directories(dataRoot, "org", "workspace", "task")
	checkout := filepath.Join(workDir, "repositories", "github", "repo")
	initInspectorCheckout(t, checkout)
	if err := os.MkdirAll(filepath.Join(checkout, ".git", "objects", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, ".git", "objects", "info", "alternates"), []byte("/outside/objects\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inspector := TaskCheckoutInspector{DataRoot: dataRoot}
	if _, err := inspector.Inspect(context.Background(), "org", "workspace", "task", []taskeditor.Checkout{{RegistrationID: "repo", ProviderName: "github", Name: "repo"}}); err == nil {
		t.Fatal("expected external Git alternates rejection")
	}
}

func TestTaskCheckoutInspectorRejectsGitFileWorktreeMetadata(t *testing.T) {
	dataRoot := t.TempDir()
	workDir, _ := taskstorage.Directories(dataRoot, "org", "workspace", "task")
	checkout := filepath.Join(workDir, "repositories", "github", "repo")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, ".git"), []byte("gitdir: /outside/worktree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inspector := TaskCheckoutInspector{DataRoot: dataRoot}
	if _, err := inspector.Inspect(context.Background(), "org", "workspace", "task", []taskeditor.Checkout{{RegistrationID: "repo", ProviderName: "github", Name: "repo"}}); err == nil {
		t.Fatal("expected linked worktree metadata rejection")
	}
}

func TestTaskCheckoutInspectorRejectsInvalidIdentityAndHonorsCancellation(t *testing.T) {
	inspector := TaskCheckoutInspector{DataRoot: t.TempDir()}
	if _, err := inspector.Inspect(context.Background(), "org", "workspace", "task", []taskeditor.Checkout{{RegistrationID: "../escape", ProviderName: "github", Name: "repo"}}); err == nil {
		t.Fatal("expected invalid identity rejection")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := inspector.Inspect(ctx, "org", "workspace", "task", []taskeditor.Checkout{{RegistrationID: "r", ProviderName: "github", Name: "repo"}}); err == nil {
		t.Fatal("expected canceled inspection")
	}
}

func initInspectorCheckout(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, path, "init", "--quiet")
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return string(output)
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}
