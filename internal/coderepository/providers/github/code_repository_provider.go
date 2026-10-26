package github

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/coderepository"
)

const ProviderName = "github"

type GitHubCodeRepositoryProvider struct{}

var _ coderepository.CodeRepositoryProvider = GitHubCodeRepositoryProvider{}

func NewGitHubCodeRepositoryProvider() GitHubCodeRepositoryProvider {
	return GitHubCodeRepositoryProvider{}
}

func (GitHubCodeRepositoryProvider) ProviderName() string { return ProviderName }

func (GitHubCodeRepositoryProvider) Clone(ctx context.Context, repositoryURL, path string) error {
	return runGit(ctx, "clone", "--", repositoryURL, path)
}

func (p GitHubCodeRepositoryProvider) PrepareTaskCheckout(ctx context.Context, repositoryURL, path, branch string) error {
	if strings.TrimSpace(branch) == "" || strings.ContainsAny(branch, "\x00\r\n") {
		return fmt.Errorf("task branch is invalid")
	}
	if _, err := os.Stat(filepath.Join(path, ".git")); errors.Is(err, os.ErrNotExist) {
		if err = p.Clone(ctx, repositoryURL, path); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("inspect task checkout: %w", err)
	} else {
		remoteURL, remoteErr := gitOutput(ctx, path, "remote", "get-url", "origin")
		if remoteErr != nil {
			return fmt.Errorf("read task checkout origin: %w", remoteErr)
		}
		if canonicalURL(remoteURL) != canonicalURL(repositoryURL) {
			return fmt.Errorf("task checkout origin does not match registered repository")
		}
	}
	if err := runGit(ctx, "-C", path, "check-ref-format", "--branch", branch); err != nil {
		return fmt.Errorf("validate task branch: %w", err)
	}
	if _, err := gitOutput(ctx, path, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		return runGit(ctx, "-C", path, "checkout", branch)
	}
	return runGit(ctx, "-C", path, "checkout", "-b", branch)
}

func (GitHubCodeRepositoryProvider) SyncMain(ctx context.Context, repositoryURL, path string) error {
	remoteURL, err := gitOutput(ctx, path, "remote", "get-url", "origin")
	if err != nil {
		return fmt.Errorf("read origin URL: %w", err)
	}
	if canonicalURL(remoteURL) != canonicalURL(repositoryURL) {
		return fmt.Errorf("checkout origin does not match registered repository")
	}
	if err = runGit(ctx, "-C", path, "fetch", "origin", "main"); err != nil {
		return fmt.Errorf("fetch main: %w", err)
	}
	if err = runGit(ctx, "-C", path, "reset", "--hard", "FETCH_HEAD"); err != nil {
		return fmt.Errorf("reset to fetched main: %w", err)
	}
	return nil
}

func runGit(ctx context.Context, args ...string) error {
	command := exec.CommandContext(ctx, "git", args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s failed: %w: %s", args[0], err, strings.TrimSpace(string(output)))
	}
	return nil
}

func gitOutput(ctx context.Context, path string, args ...string) (string, error) {
	commandArgs := append([]string{"-C", path}, args...)
	command := exec.CommandContext(ctx, "git", commandArgs...)
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w", args[0], err)
	}
	return strings.TrimSpace(string(output)), nil
}

func canonicalURL(value string) string {
	return strings.TrimSuffix(strings.TrimSpace(value), ".git")
}
