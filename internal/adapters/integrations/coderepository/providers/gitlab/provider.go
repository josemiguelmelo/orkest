package gitlab

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

	gitlabsdk "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/josemiguelmelo/orkest/internal/server/coderepository"
	coderepositorymodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
)

const (
	CloudProviderName  = "gitlab_cloud"
	ServerProviderName = "gitlab_server"
	cloudHost          = "https://gitlab.com"
)

type Provider struct {
	Name   string
	Client *http.Client
}

var _ coderepository.CodingProvider = Provider{}

func NewGitLabCloudProvider() Provider {
	return Provider{Name: CloudProviderName}
}

func NewGitLabServerProvider() Provider { return Provider{Name: ServerProviderName} }

func (p Provider) ProviderName() string { return p.Name }

func (p Provider) ConfigSchema() json.RawMessage {
	if p.Name == CloudProviderName {
		return json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	}
	return json.RawMessage(`{"type":"object","required":["baseUrl"],"properties":{"baseUrl":{"type":"string","format":"uri","title":"GitLab Server URL","description":"HTTPS base URL of your self-hosted GitLab instance."}},"additionalProperties":false}`)
}

func (p Provider) ValidateConfig(raw json.RawMessage) error {
	if len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage(`{}`)
	}
	var cfg map[string]string
	if json.Unmarshal(raw, &cfg) != nil {
		return errors.New("GitLab configuration is invalid")
	}
	if p.Name == CloudProviderName {
		if len(cfg) != 0 {
			return errors.New("GitLab Cloud configuration is invalid")
		}
		return nil
	}
	if len(cfg) != 1 || strings.TrimSpace(cfg["baseUrl"]) == "" {
		return errors.New("GitLab Server URL is required")
	}
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(cfg["baseUrl"]), "/"))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("GitLab Server URL must be an HTTPS base URL")
	}
	return nil
}

func (p Provider) baseURL(raw json.RawMessage) (string, error) {
	if err := p.ValidateConfig(raw); err != nil {
		return "", err
	}
	if p.Name == CloudProviderName {
		return cloudHost, nil
	}
	var cfg struct {
		BaseURL string `json:"baseUrl"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", errors.New("GitLab configuration is invalid")
	}
	return strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"), nil
}

func (p Provider) ValidateAuthentication(ctx context.Context, cfg json.RawMessage, credential coderepositorymodel.Credentials) error {
	base, err := p.baseURL(cfg)
	if err != nil {
		return err
	}
	if strings.TrimSpace(credential.Token) == "" {
		return &coderepositorymodel.ProviderError{Code: "gitlab_token_required", Status: http.StatusBadRequest}
	}
	client, err := p.sdkClient(base, credential.Token)
	if err != nil {
		return providerError("gitlab_connection_failed", http.StatusBadGateway)
	}
	_, response, err := client.Users.CurrentUser(gitlabsdk.WithContext(ctx))
	if err != nil {
		if response != nil && (response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden) {
			return providerError("gitlab_authentication_failed", http.StatusBadGateway)
		}
		return providerError("gitlab_connection_failed", http.StatusBadGateway)
	}
	return nil
}

func (p Provider) DiscoverRepositories(ctx context.Context, cfg json.RawMessage, credential coderepositorymodel.Credentials, page, perPage int) (coderepositorymodel.RepositoryPage, error) {
	base, err := p.baseURL(cfg)
	if err != nil {
		return coderepositorymodel.RepositoryPage{}, err
	}
	if strings.TrimSpace(credential.Token) == "" {
		return coderepositorymodel.RepositoryPage{}, providerError("gitlab_token_required", http.StatusBadRequest)
	}
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 50
	}
	client, err := p.sdkClient(base, credential.Token)
	if err != nil {
		return coderepositorymodel.RepositoryPage{}, providerError("gitlab_repository_request_failed", http.StatusBadGateway)
	}
	entries, response, err := client.Projects.ListProjects(&gitlabsdk.ListProjectsOptions{
		Membership:  new(true),
		Simple:      new(true),
		OrderBy:     new("name"),
		Sort:        new("asc"),
		ListOptions: gitlabsdk.ListOptions{Page: int64(page), PerPage: int64(perPage)},
	}, gitlabsdk.WithContext(ctx))
	if err != nil {
		code := "gitlab_repository_request_failed"
		if response != nil {
			switch response.StatusCode {
			case http.StatusUnauthorized:
				code = "gitlab_token_invalid_or_revoked"
			case http.StatusForbidden:
				code = "gitlab_permission_denied"
			case http.StatusOK:
				code = "gitlab_repository_response_invalid"
			}
		}
		// Never return SDK errors containing provider response bodies or request details.
		return coderepositorymodel.RepositoryPage{}, providerError(code, http.StatusBadGateway)
	}
	result := coderepositorymodel.RepositoryPage{Repositories: make([]coderepositorymodel.RepositoryMetadata, 0, len(entries)), NextPage: int(response.NextPage)}
	for _, item := range entries {
		if item == nil {
			return coderepositorymodel.RepositoryPage{}, providerError("gitlab_repository_response_invalid", http.StatusBadGateway)
		}
		if err := p.validateCloneURL(base, item.HTTPURLToRepo); err != nil {
			return coderepositorymodel.RepositoryPage{}, providerError("gitlab_repository_response_invalid", http.StatusBadGateway)
		}
		result.Repositories = append(result.Repositories, coderepositorymodel.RepositoryMetadata{ID: strconv.FormatInt(item.ID, 10), Name: item.PathWithNamespace, CloneURL: item.HTTPURLToRepo, Private: item.Visibility != gitlabsdk.PublicVisibility, DefaultBranch: item.DefaultBranch})
	}
	// Some self-hosted endpoints omit pagination headers; retain the previous fallback.
	_, hasNextPage := response.Header["X-Next-Page"]
	if !hasNextPage && response.Header.Get("X-Page") == "" && response.Header.Get("Link") == "" && len(entries) == perPage {
		result.NextPage = page + 1
	}
	return result, nil
}

func (p Provider) validateCloneURL(base, cloneURL string) error {
	baseURL, err := url.Parse(base)
	if err != nil {
		return errors.New("GitLab base URL is invalid")
	}
	clone, err := url.Parse(cloneURL)
	if err != nil {
		return errors.New("GitLab repository URL is invalid")
	}
	if _, err = p.NormalizeRepositoryURL(cloneURL); err != nil || !strings.EqualFold(baseURL.Host, clone.Host) {
		return errors.New("GitLab repository URL does not match configured instance")
	}
	basePath := strings.TrimRight(baseURL.Path, "/")
	if basePath != "" && !strings.HasPrefix(clone.Path, basePath+"/") {
		return errors.New("GitLab repository URL does not match configured instance")
	}
	return nil
}

func (p Provider) ResolveRepository(ctx context.Context, cfg json.RawMessage, credential coderepositorymodel.Credentials, id string) (coderepositorymodel.RepositoryMetadata, error) {
	for page := 1; page <= 100; page++ {
		result, err := p.DiscoverRepositories(ctx, cfg, credential, page, 100)
		if err != nil {
			return coderepositorymodel.RepositoryMetadata{}, err
		}
		for _, repo := range result.Repositories {
			if repo.ID == id {
				return repo, nil
			}
		}
		if result.NextPage == 0 {
			break
		}
	}
	return coderepositorymodel.RepositoryMetadata{}, providerError("gitlab_repository_unavailable", http.StatusNotFound)
}

func (Provider) NormalizeRepositoryURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	path := strings.Trim(strings.TrimSuffix(parsedPath(parsed), ".git"), "/")
	segments := strings.Split(path, "/")
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || len(segments) < 2 || strings.Contains(path, "/-/") {
		return "", errors.New("GitLab repository URL is invalid")
	}
	return "https://" + parsed.Host + "/" + path + ".git", nil
}

func parsedPath(u *url.URL) string {
	if u == nil {
		return ""
	}
	return u.Path
}

func (p Provider) CloneAuthenticated(ctx context.Context, repo coderepositorymodel.RepositoryMetadata, credential coderepositorymodel.Credentials, path string) error {
	return p.git(ctx, repo, credential, path, "clone")
}

func (p Provider) SyncAuthenticated(ctx context.Context, repo coderepositorymodel.RepositoryMetadata, credential coderepositorymodel.Credentials, path string) error {
	return p.git(ctx, repo, credential, path, "sync")
}

func (p Provider) PrepareAuthenticatedTaskCheckout(ctx context.Context, repo coderepositorymodel.RepositoryMetadata, credential coderepositorymodel.Credentials, path, branch string) error {
	if branch == "" {
		return p.git(ctx, repo, credential, path, "task")
	}
	return p.git(ctx, repo, credential, path, "task", branch)
}

func (p Provider) git(ctx context.Context, repo coderepositorymodel.RepositoryMetadata, credential coderepositorymodel.Credentials, path, operation string, branch ...string) error {
	canonical, err := p.NormalizeRepositoryURL(repo.CloneURL)
	if err != nil || canonicalURL(canonical) != canonicalURL(repo.CloneURL) {
		return errors.New("registered GitLab origin is invalid")
	}
	if strings.TrimSpace(credential.Token) == "" {
		return errors.New("GitLab token is required")
	}
	if operation == "clone" {
		if _, e := os.Stat(filepath.Join(path, ".git")); e == nil {
			origin, e := gitOutput(ctx, path, "remote", "get-url", "origin")
			if e != nil || canonicalURL(origin) != canonicalURL(canonical) {
				return errors.New("checkout origin does not match registered repository")
			}
			return nil
		} else if !errors.Is(e, os.ErrNotExist) {
			return errors.New("inspect checkout origin failed")
		}
	}
	if operation != "clone" {
		origin, e := gitOutput(ctx, path, "remote", "get-url", "origin")
		if e == nil && canonicalURL(origin) != canonicalURL(canonical) {
			return errors.New("checkout origin does not match registered repository")
		}
		if e != nil && operation != "task" {
			return errors.New("checkout origin is unavailable")
		}
	}
	if operation == "task" {
		if len(branch) > 1 || (len(branch) == 1 && (strings.TrimSpace(branch[0]) == "" || strings.ContainsAny(branch[0], "\x00\r\n"))) {
			return errors.New("task branch is invalid")
		}
		if _, e := os.Stat(filepath.Join(path, ".git")); errors.Is(e, os.ErrNotExist) {
			operation = "clone-task"
		} else if e != nil {
			return errors.New("inspect task checkout failed")
		}
	}
	if operation == "sync" && strings.TrimSpace(repo.DefaultBranch) == "" {
		return errors.New("repository default branch is unavailable")
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errors.New("create repository directory failed")
	}
	dir, err := os.MkdirTemp("", "orkest-gitlab-credential-")
	if err != nil {
		return errors.New("prepare Git credential helper failed")
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err = os.WriteFile(filepath.Join(dir, "token"), []byte(credential.Token), 0o600); err != nil {
		return errors.New("prepare Git credential helper failed")
	}
	helper := filepath.Join(dir, "helper.sh")
	script := "#!/bin/sh\ntoken_file=\"${0%/*}/token\"\nprintf 'username=oauth2\\npassword=%s\\n' \"$(cat \"$token_file\")\"\n"
	if err = os.WriteFile(helper, []byte(script), 0o700); err != nil {
		return errors.New("prepare Git credential helper failed")
	}
	prefix := []string{"-c", "credential.helper=", "-c", "credential.helper=" + helper, "-c", "credential.useHttpPath=true"}
	args := []string{}
	switch operation {
	case "clone", "clone-task":
		args = []string{"clone", "--", canonical, path}
	case "sync":
		args = []string{"-C", path, "fetch", "origin", repo.DefaultBranch}
	case "task":
		if len(branch) == 1 {
			if err = runGit(ctx, prefix, "-C", path, "check-ref-format", "--branch", branch[0]); err != nil {
				return errors.New("task branch is invalid")
			}
			if _, e := gitOutput(ctx, path, "show-ref", "--verify", "--quiet", "refs/heads/"+branch[0]); e == nil {
				args = []string{"-C", path, "checkout", branch[0]}
			} else {
				args = []string{"-C", path, "checkout", "-b", branch[0]}
			}
		}
	}
	if len(args) > 0 {
		if err = runGit(ctx, prefix, args...); err != nil {
			return fmt.Errorf("GitLab repository operation failed")
		}
	}
	if operation == "sync" {
		if err = runGit(ctx, prefix, "-C", path, "reset", "--hard", "FETCH_HEAD"); err != nil {
			return errors.New("GitLab repository sync failed")
		}
	}
	if operation == "clone-task" && len(branch) == 1 {
		if err = runGit(ctx, prefix, "-C", path, "check-ref-format", "--branch", branch[0]); err != nil {
			return errors.New("task branch is invalid")
		}
		if err = runGit(ctx, prefix, "-C", path, "checkout", "-b", branch[0]); err != nil {
			return errors.New("GitLab task checkout preparation failed")
		}
	}
	return nil
}

func (p Provider) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (p Provider) sdkClient(base, token string) (*gitlabsdk.Client, error) {
	return gitlabsdk.NewClient(token, gitlabsdk.WithBaseURL(base), gitlabsdk.WithHTTPClient(p.client()), gitlabsdk.WithoutRetries())
}

func providerError(code string, status int) error {
	return &coderepositorymodel.ProviderError{Code: code, Status: status}
}

func canonicalURL(value string) string { return strings.TrimSuffix(strings.TrimSpace(value), "/") }

func gitOutput(ctx context.Context, path string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", path}, args...)...)
	out, err := command.Output()
	return strings.TrimSpace(string(out)), err
}

func runGit(ctx context.Context, prefix []string, args ...string) error {
	command := exec.CommandContext(ctx, "git", append(append([]string{}, prefix...), args...)...)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=Never")
	if _, err := command.CombinedOutput(); err != nil {
		return errors.New("git operation failed")
	}
	return nil
}
