package github

import (
	"context"
	"fmt"
	"os/exec"
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
