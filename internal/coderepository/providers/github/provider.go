package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/josemiguelmelo/orkest/internal/coderepository"
)

const ProviderName = "github"

type GitHubProvider struct{ Client *http.Client }

var _ coderepository.CodingProvider = GitHubProvider{}

func NewGitHubProvider() GitHubProvider {
	return GitHubProvider{Client: &http.Client{Timeout: 20 * time.Second}}
}

func (GitHubProvider) ProviderName() string { return ProviderName }

func (GitHubProvider) ConfigSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"host":{"type":"string","enum":["github.com"],"default":"github.com"}},"additionalProperties":false}`)
}

func (GitHubProvider) ValidateConfig(raw json.RawMessage) error {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return nil
	}
	var cfg map[string]string
	if json.Unmarshal(raw, &cfg) != nil || (cfg["host"] != "" && cfg["host"] != "github.com") {
		return errors.New("GitHub configuration is invalid")
	}
	return nil
}

func (p GitHubProvider) ValidateAuthentication(ctx context.Context, cfg json.RawMessage, credential coderepository.Credentials) error {
	if err := p.ValidateConfig(cfg); err != nil {
		return err
	}
	if strings.TrimSpace(credential.Token) == "" {
		return errors.New("GitHub token is required")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return errors.New("GitHub connection failed")
	}
	request.Header.Set("Authorization", "Bearer "+credential.Token)
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := p.client().Do(request)
	if err != nil {
		return errors.New("GitHub connection failed")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return errors.New("GitHub authentication or permission check failed")
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return errors.New("GitHub connection failed")
	}
	return nil
}

func (p GitHubProvider) DiscoverRepositories(ctx context.Context, cfg json.RawMessage, credential coderepository.Credentials, page, perPage int) (coderepository.RepositoryPage, error) {
	if err := p.ValidateConfig(cfg); err != nil {
		return coderepository.RepositoryPage{}, err
	}
	if strings.TrimSpace(credential.Token) == "" {
		return coderepository.RepositoryPage{}, errors.New("GitHub token is required")
	}
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 50
	}
	endpoint := "https://api.github.com/user/repos?page=" + strconv.Itoa(page) + "&per_page=" + strconv.Itoa(perPage) + "&sort=full_name&affiliation=owner,collaborator,organization_member"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return coderepository.RepositoryPage{}, errors.New("GitHub repository request failed")
	}
	request.Header.Set("Authorization", "Bearer "+credential.Token)
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := p.client().Do(request)
	if err != nil {
		return coderepository.RepositoryPage{}, errors.New("GitHub repository request failed")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == 401 {
		return coderepository.RepositoryPage{}, errors.New("GitHub token is invalid or revoked")
	}
	if response.StatusCode == 403 {
		return coderepository.RepositoryPage{}, errors.New("GitHub denied repository access or rate limit was reached")
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return coderepository.RepositoryPage{}, errors.New("GitHub repository request failed")
	}
	var entries []struct {
		ID            int64  `json:"id"`
		Name          string `json:"full_name"`
		CloneURL      string `json:"clone_url"`
		Private       bool   `json:"private"`
		DefaultBranch string `json:"default_branch"`
	}
	if json.NewDecoder(response.Body).Decode(&entries) != nil {
		return coderepository.RepositoryPage{}, errors.New("GitHub repository response was invalid")
	}
	result := coderepository.RepositoryPage{Repositories: make([]coderepository.RepositoryMetadata, 0, len(entries))}
	for _, entry := range entries {
		result.Repositories = append(result.Repositories, coderepository.RepositoryMetadata{ID: strconv.FormatInt(entry.ID, 10), Name: entry.Name, CloneURL: entry.CloneURL, Private: entry.Private, DefaultBranch: entry.DefaultBranch})
	}
	if len(entries) == perPage {
		result.NextPage = page + 1
	}
	return result, nil
}

func (p GitHubProvider) ResolveRepository(ctx context.Context, cfg json.RawMessage, cred coderepository.Credentials, id string) (coderepository.RepositoryMetadata, error) {
	for page := 1; page <= 100; page++ {
		result, err := p.DiscoverRepositories(ctx, cfg, cred, page, 100)
		if err != nil {
			return coderepository.RepositoryMetadata{}, err
		}
		for _, item := range result.Repositories {
			if item.ID == id {
				return item, nil
			}
		}
		if result.NextPage == 0 {
			break
		}
	}
	return coderepository.RepositoryMetadata{}, errors.New("GitHub repository is unavailable to this integration")
}

func (GitHubProvider) NormalizeRepositoryURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || len(strings.Split(strings.Trim(parsed.Path, "/"), "/")) != 2 {
		return "", errors.New("GitHub repository URL is invalid")
	}
	clean := "https://github.com/" + strings.TrimSuffix(strings.Trim(parsed.Path, "/"), ".git") + ".git"
	return clean, nil
}

func (p GitHubProvider) CloneAuthenticated(ctx context.Context, repo coderepository.RepositoryMetadata, credential coderepository.Credentials, path string) error {
	return p.gitAuthenticated(ctx, repo, credential, path, "clone")
}

func (p GitHubProvider) SyncAuthenticated(ctx context.Context, repo coderepository.RepositoryMetadata, credential coderepository.Credentials, path string) error {
	return p.gitAuthenticated(ctx, repo, credential, path, "sync")
}

func (p GitHubProvider) PrepareAuthenticatedTaskCheckout(ctx context.Context, repo coderepository.RepositoryMetadata, credential coderepository.Credentials, path, branch string) error {
	return p.gitAuthenticated(ctx, repo, credential, path, "task", branch)
}

func (p GitHubProvider) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (p GitHubProvider) gitAuthenticated(ctx context.Context, repo coderepository.RepositoryMetadata, credential coderepository.Credentials, path, operation string, branch ...string) error {
	if strings.TrimSpace(credential.Token) == "" {
		return errors.New("GitHub token is required")
	}
	canonical, err := p.NormalizeRepositoryURL(repo.CloneURL)
	if err != nil || canonicalURL(canonical) != canonicalURL(repo.CloneURL) {
		return errors.New("registered GitHub origin is invalid")
	}
	if operation != "clone" {
		origin, originErr := gitOutput(ctx, path, "remote", "get-url", "origin")
		if originErr == nil && canonicalURL(origin) != canonicalURL(canonical) {
			return errors.New("checkout origin does not match registered repository")
		}
		if originErr != nil && operation != "task" {
			return errors.New("checkout origin is unavailable")
		}
	}
	if operation == "task" {
		if len(branch) != 1 || strings.TrimSpace(branch[0]) == "" || strings.ContainsAny(branch[0], "\x00\r\n") {
			return errors.New("task branch is invalid")
		}
		if _, statErr := os.Stat(filepath.Join(path, ".git")); errors.Is(statErr, os.ErrNotExist) {
			operation = "clone-task"
		} else if statErr != nil {
			return errors.New("inspect task checkout failed")
		}
	}
	if operation == "sync" && strings.TrimSpace(repo.DefaultBranch) == "" {
		return errors.New("repository default branch is unavailable")
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errors.New("create repository directory failed")
	}
	credentialDir, err := os.MkdirTemp("", "orkest-git-credential-")
	if err != nil {
		return errors.New("prepare Git credential helper failed")
	}
	defer func() { _ = os.RemoveAll(credentialDir) }()
	helper := filepath.Join(credentialDir, "helper.sh")
	script := "#!/bin/sh\nprintf 'username=x-access-token\\npassword=%s\\n' \"$(cat '" + filepath.Join(credentialDir, "token") + "')\"\n"
	if err = os.WriteFile(filepath.Join(credentialDir, "token"), []byte(credential.Token), 0o600); err != nil {
		return errors.New("prepare Git credential helper failed")
	}
	if err = os.WriteFile(helper, []byte(script), 0o700); err != nil {
		return errors.New("prepare Git credential helper failed")
	}
	args := []string{"-c", "credential.helper=" + helper, "-c", "credential.useHttpPath=true"}
	cmd := []string{}
	switch operation {
	case "clone":
		cmd = append(cmd, "clone", "--", canonical, path)
	case "clone-task":
		cmd = append(cmd, "clone", "--", canonical, path)
	case "sync":
		cmd = append(cmd, "-C", path, "fetch", "origin", repo.DefaultBranch)
		if err = runGitWithPrefix(ctx, args, cmd...); err != nil {
			return errors.New("GitHub repository sync failed")
		}
		cmd = []string{"-C", path, "reset", "--hard", "FETCH_HEAD"}
	case "task":
		if err = runGitWithPrefix(ctx, args, "-C", path, "check-ref-format", "--branch", branch[0]); err != nil {
			return errors.New("task branch is invalid")
		}
		if _, e := gitOutput(ctx, path, "show-ref", "--verify", "--quiet", "refs/heads/"+branch[0]); e == nil {
			cmd = append(cmd, "-C", path, "checkout", branch[0])
		} else {
			cmd = append(cmd, "-C", path, "checkout", "-b", branch[0])
		}
	}
	if err = runGitWithPrefix(ctx, args, cmd...); err != nil {
		return errors.New("GitHub repository operation failed")
	}
	return nil
}

func runGitWithPrefix(ctx context.Context, prefix []string, args ...string) error {
	full := append(append([]string{}, prefix...), args...)
	command := exec.CommandContext(ctx, "git", full...)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=Never")
	if _, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("git operation failed")
	}
	return nil
}

func (GitHubProvider) Clone(ctx context.Context, repositoryURL, path string) error {
	return runGit(ctx, "clone", "--", repositoryURL, path)
}

func (p GitHubProvider) PrepareTaskCheckout(ctx context.Context, repositoryURL, path, branch string) error {
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

func (GitHubProvider) Sync(ctx context.Context, repositoryURL, path string) error {
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
