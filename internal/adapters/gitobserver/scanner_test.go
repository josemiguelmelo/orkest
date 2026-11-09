package gitobserver

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	observation "github.com/josemiguelmelo/orkest/internal/contracts/repositoryobservation"
)

func TestScanRepositoriesWorktreesPackedRefsAndUnknownClone(t *testing.T) {
	root := t.TempDir()
	repository := filepath.Join(root, "repositories", "github", "repo")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	runGit := func(path string, args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", path}, args...)...)
		command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s %v", args, out, err)
		}
	}
	runGit(repository, "init", "-b", "main")
	runGit(repository, "commit", "--allow-empty", "-m", "initial")
	runGit(repository, "branch", "chosen-by-skill")
	runGit(repository, "tag", "chosen-by-skill")
	runGit(repository, "pack-refs", "--all")
	runGit(repository, "worktree", "add", "-b", "another-skill-branch", filepath.Join(root, "worktree"))
	unknown := filepath.Join(root, "unknown")
	if err := os.MkdirAll(unknown, 0o700); err != nil {
		t.Fatal(err)
	}
	runGit(unknown, "init", "-b", "main")
	manifest, _ := json.Marshal([]observation.Checkout{{RepositoryID: "repo", Provider: "github", ProviderRepositoryID: "123", Path: "repositories/github/repo", DefaultBranch: "main", RepositoryURL: "https://github.com/owner/repo"}})
	if err := os.WriteFile(filepath.Join(root, observation.ManifestName), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshots, err := (Scanner{Root: root}).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := make(map[string]observation.Snapshot)
	for _, snapshot := range snapshots {
		found[snapshot.Checkout.Path] = snapshot
	}
	hasExactBranch := false
	for _, branch := range found["repositories/github/repo"].Branches {
		if branch.Name == "chosen-by-skill" {
			hasExactBranch = true
		}
	}
	if !hasExactBranch {
		t.Fatal("branch name changed due to matching tag")
	}
	if len(found["repositories/github/repo"].Branches) != 3 {
		t.Fatalf("packed branches missing: %+v", snapshots)
	}
	if found["unknown"].Diagnostic != "unregistered_repository" {
		t.Fatalf("unknown repository claimed: %+v", snapshots)
	}
	if len(found["worktree"].Branches) != 3 {
		t.Fatalf("worktree not observed: %+v", snapshots)
	}
}

func TestScanDoesNotFollowCheckoutOutsideTask(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.Marshal([]observation.Checkout{{RepositoryID: "repo", Path: "escape"}})
	if err := os.WriteFile(filepath.Join(root, observation.ManifestName), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	snapshots, err := (Scanner{Root: root}).Scan(context.Background())
	if err != nil || len(snapshots) != 1 || snapshots[0].Diagnostic != "checkout_unavailable" {
		t.Fatalf("outside task inspected: %+v %v", snapshots, err)
	}
}
