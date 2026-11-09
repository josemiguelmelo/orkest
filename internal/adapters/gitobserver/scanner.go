// Package gitobserver inspects local Git state without fetching or mutating repositories.
package gitobserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	observation "github.com/josemiguelmelo/orkest/internal/contracts/repositoryobservation"
)

type Scanner struct{ Root string }

func (s Scanner) Scan(ctx context.Context) ([]observation.Snapshot, error) {
	root, err := filepath.EvalSymlinks(s.Root)
	if err != nil {
		return nil, errors.New("task observation root unavailable")
	}
	data, err := os.ReadFile(filepath.Join(root, observation.ManifestName))
	if errors.Is(err, os.ErrNotExist) {
		return []observation.Snapshot{{Checkout: observation.Checkout{Path: "."}, Branches: []observation.Branch{}, Diagnostic: "checkout_manifest_unavailable"}}, nil
	}
	if err != nil || len(data) > 1024*1024 {
		return nil, errors.New("checkout observation manifest unavailable")
	}
	var checkouts []observation.Checkout
	if json.Unmarshal(data, &checkouts) != nil {
		return nil, errors.New("invalid checkout observation manifest")
	}
	known := make(map[string]observation.Checkout)
	for _, checkout := range checkouts {
		known[filepath.Clean(checkout.Path)] = checkout
	}
	paths, err := discoverCheckoutPaths(ctx, root, known)
	if err != nil {
		return nil, err
	}
	var result []observation.Snapshot
	for path, checkout := range paths {
		result = append(result, inspectCheckout(ctx, root, path, checkout, checkouts))
	}
	return result, nil
}

func discoverCheckoutPaths(ctx context.Context, root string, known map[string]observation.Checkout) (map[string]observation.Checkout, error) {
	paths := make(map[string]observation.Checkout)
	visited := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return errors.New("repository discovery incomplete")
		}
		visited++
		if visited > 100000 {
			return errors.New("repository discovery limit exceeded")
		}
		relative, _ := filepath.Rel(root, path)
		if entry.IsDir() && (entry.Name() == "node_modules" || strings.Count(relative, string(filepath.Separator)) > 12) {
			return filepath.SkipDir
		}
		if entry.Name() != ".git" {
			return nil
		}
		directory := filepath.Dir(path)
		rel, _ := filepath.Rel(root, directory)
		checkout, registered := known[rel]
		if !registered {
			checkout = observation.Checkout{Path: rel}
		}
		paths[directory] = checkout
		if entry.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for path, checkout := range known {
		absolute := filepath.Join(root, path)
		if contained(root, absolute) {
			paths[absolute] = checkout
		}
	}
	return paths, nil
}

func inspectCheckout(ctx context.Context, root, path string, checkout observation.Checkout, checkouts []observation.Checkout) observation.Snapshot {
	snapshot := observation.Snapshot{Checkout: checkout}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || !contained(root, resolved) {
		snapshot.Diagnostic = "checkout_unavailable"
		return snapshot
	}
	gitDir, err := git(ctx, path, "rev-parse", "--absolute-git-dir")
	resolvedGit, resolveErr := filepath.EvalSymlinks(strings.TrimSpace(gitDir))
	if err != nil || resolveErr != nil || !contained(root, resolvedGit) {
		snapshot.Diagnostic = "git_directory_outside_task_or_unavailable"
		return snapshot
	}
	remote, _ := git(ctx, path, "config", "--get", "remote.origin.url")
	// Do not put arbitrary remote URLs (which may contain credentials) on the wire.
	if checkout.RepositoryID == "" {
		for _, candidate := range checkouts {
			if strings.TrimSuffix(strings.TrimSpace(remote), ".git") == strings.TrimSuffix(candidate.RepositoryURL, ".git") {
				checkout = candidate
				checkout.Path, _ = filepath.Rel(root, path)
				snapshot.Checkout = checkout
				break
			}
		}
	}
	if snapshot.Checkout.RepositoryID == "" {
		common, commonErr := git(ctx, path, "rev-parse", "--path-format=absolute", "--git-common-dir")
		if commonErr == nil {
			for _, candidate := range checkouts {
				expected := filepath.Join(root, candidate.Path, ".git")
				if resolvedExpected, e := filepath.EvalSymlinks(expected); e == nil && filepath.Clean(strings.TrimSpace(common)) == resolvedExpected {
					candidate.Path, _ = filepath.Rel(root, path)
					snapshot.Checkout = candidate
					break
				}
			}
		}
	}
	if snapshot.Checkout.RepositoryID == "" {
		snapshot.Diagnostic = "unregistered_repository"
	}
	refs, err := git(ctx, path, "for-each-ref", "--format=%(refname:lstrip=2) %(objectname)", "refs/heads/")
	if err != nil {
		snapshot.Diagnostic = "git_inspection_failed"
		return snapshot
	}
	for _, line := range strings.Split(strings.TrimSpace(refs), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 {
			snapshot.Branches = append(snapshot.Branches, observation.Branch{Name: fields[0], CommitID: fields[1]})
		}
	}
	return snapshot
}

func contained(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func git(ctx context.Context, directory string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", append([]string{"-C", directory, "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("inspect Git repository: command failed")
	}
	return string(output), nil
}
