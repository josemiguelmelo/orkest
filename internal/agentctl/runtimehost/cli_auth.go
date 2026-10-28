package runtimehost

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

// CLIAuthAdapter adapts provider credentials to a provider-specific CLI without
// coupling runtime startup to any provider's host or environment variables.
type CLIAuthAdapter interface {
	ProviderNames() []string
	Configure(string, []string, []runconfig.GitCredential) ([]string, error)
}

// DefaultCLIAuthAdapters returns the CLI strategies shipped with agentctl.
func DefaultCLIAuthAdapters() []CLIAuthAdapter {
	return []CLIAuthAdapter{GitHubCLIAuthAdapter{}, GitLabCLIAuthAdapter{}}
}

func credentialsForProviders(credentials []runconfig.GitCredential, providers []string) []runconfig.GitCredential {
	allowed := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		allowed[provider] = struct{}{}
	}
	matched := make([]runconfig.GitCredential, 0, len(credentials))
	for _, credential := range credentials {
		if _, ok := allowed[credential.ProviderName]; ok {
			matched = append(matched, credential)
		}
	}
	return matched
}

func configureProviderCLIs(runtimeRoot string, environment []string, credentials []runconfig.GitCredential, adapters []CLIAuthAdapter) ([]string, error) {
	var err error
	for _, adapter := range adapters {
		matched := credentialsForProviders(credentials, adapter.ProviderNames())
		if len(matched) == 0 {
			continue
		}
		environment, err = adapter.Configure(runtimeRoot, environment, matched)
		if err != nil {
			return nil, err
		}
	}
	return environment, nil
}

// GitHubCLIAuthAdapter scopes gh's token to the registered checkout origin.
type GitHubCLIAuthAdapter struct{}

func (GitHubCLIAuthAdapter) ProviderNames() []string { return []string{"github"} }

func (GitHubCLIAuthAdapter) Configure(runtimeRoot string, environment []string, credentials []runconfig.GitCredential) ([]string, error) {
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		return nil, &agentruntime.LaunchError{Message: "GitHub CLI is unavailable", Cause: err}
	}
	binDir := filepath.Join(runtimeRoot, "github-cli")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		return nil, &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "configure GitHub CLI authentication failed", Cause: err}
	}
	var script strings.Builder
	script.WriteString("#!/bin/sh\nset -eu\nunset GH_TOKEN GITHUB_TOKEN GH_ENTERPRISE_TOKEN GITHUB_ENTERPRISE_TOKEN GH_HOST\n")
	script.WriteString("repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || { echo 'gh requires a registered repository checkout' >&2; exit 1; }\n")
	script.WriteString("remote=$(git -C \"$repo_root\" remote get-url origin 2>/dev/null) || { echo 'gh could not read this checkout origin' >&2; exit 1; }\n")
	script.WriteString("case \"$remote\" in\n")
	for index, credential := range credentials {
		repository, err := url.Parse(credential.RepositoryURL)
		if err != nil || repository.Scheme != "https" || !strings.EqualFold(repository.Hostname(), "github.com") {
			return nil, &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "configure GitHub CLI authentication failed", Cause: errors.New("GitHub CLI repository host is invalid")}
		}
		canonical := strings.TrimSuffix(credential.RepositoryURL, ".git")
		fmt.Fprintf(&script, "  %s|%s) GH_TOKEN=\"${ORKEST_GH_TOKEN_%d:?GitHub credential unavailable}\"; export GH_TOKEN ;;\n", shellSingleQuote(canonical), shellSingleQuote(canonical+".git"), index)
	}
	script.WriteString("  *) echo 'gh has no credential for this checkout' >&2; exit 1 ;;\nesac\n")
	fmt.Fprintf(&script, "exec %s \"$@\"\n", shellSingleQuote(ghPath))
	if err := writeCLIWrapper(binDir, "gh", script.String()); err != nil {
		return nil, &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "configure GitHub CLI authentication failed", Cause: err}
	}
	values := environmentMap(environment, "GH_TOKEN", "GITHUB_TOKEN", "ORKEST_GH_")
	for index, credential := range credentials {
		values["ORKEST_GH_TOKEN_"+strconv.Itoa(index)] = credential.Token
	}
	values["GH_PROMPT_DISABLED"] = "1"
	return prependPath(values, binDir), nil
}

// GitLabCLIAuthAdapter configures glab per registered checkout, supporting
// both GitLab Cloud and self-managed installations, including subfolder URLs.
type GitLabCLIAuthAdapter struct{}

func (GitLabCLIAuthAdapter) ProviderNames() []string {
	return []string{"gitlab_cloud", "gitlab_server"}
}

func (GitLabCLIAuthAdapter) Configure(runtimeRoot string, environment []string, credentials []runconfig.GitCredential) ([]string, error) {
	glabPath, err := exec.LookPath("glab")
	if err != nil {
		return nil, &agentruntime.LaunchError{Message: "GitLab CLI is unavailable", Cause: err}
	}
	binDir := filepath.Join(runtimeRoot, "gitlab-cli")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		return nil, &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "configure GitLab CLI authentication failed", Cause: err}
	}
	var script strings.Builder
	script.WriteString("#!/bin/sh\nset -eu\nunset GITLAB_TOKEN GITLAB_ACCESS_TOKEN OAUTH_TOKEN GITLAB_HOST GITLAB_URI GL_HOST GITLAB_REPO GITLAB_SUBFOLDER GITLAB_API_HOST\n")
	script.WriteString("repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || { echo 'glab requires a registered repository checkout' >&2; exit 1; }\n")
	script.WriteString("remote=$(git -C \"$repo_root\" remote get-url origin 2>/dev/null) || { echo 'glab could not read this checkout origin' >&2; exit 1; }\n")
	script.WriteString("case \"$remote\" in\n")
	values := environmentMap(environment, "GITLAB_TOKEN", "GITLAB_ACCESS_TOKEN", "OAUTH_TOKEN", "GITLAB_HOST", "GITLAB_URI", "GL_HOST", "GITLAB_REPO", "GITLAB_SUBFOLDER", "GITLAB_API_HOST", "GLAB_NO_PROMPT", "ORKEST_GITLAB_")
	for index, credential := range credentials {
		host, subfolder, err := gitLabCLIHost(credential)
		if err != nil {
			return nil, &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "configure GitLab CLI authentication failed", Cause: err}
		}
		canonical := strings.TrimSuffix(credential.RepositoryURL, ".git")
		fmt.Fprintf(&script, "  %s|%s) GITLAB_TOKEN=\"${ORKEST_GITLAB_TOKEN_%d:?GitLab credential unavailable}\"; GITLAB_HOST=%s; GITLAB_REPO=\"$remote\"; GITLAB_SUBFOLDER=%s; export GITLAB_TOKEN GITLAB_HOST GITLAB_REPO GITLAB_SUBFOLDER ;;\n", shellSingleQuote(canonical), shellSingleQuote(canonical+".git"), index, shellSingleQuote(host), shellSingleQuote(subfolder))
		values["ORKEST_GITLAB_TOKEN_"+strconv.Itoa(index)] = credential.Token
	}
	script.WriteString("  *) echo 'glab has no credential for this checkout' >&2; exit 1 ;;\nesac\n")
	fmt.Fprintf(&script, "exec %s \"$@\"\n", shellSingleQuote(glabPath))
	if err := writeCLIWrapper(binDir, "glab", script.String()); err != nil {
		return nil, &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "configure GitLab CLI authentication failed", Cause: err}
	}
	values["GLAB_NO_PROMPT"] = "1"
	return prependPath(values, binDir), nil
}

func gitLabCLIHost(credential runconfig.GitCredential) (string, string, error) {
	base := "https://gitlab.com"
	if credential.ProviderName == "gitlab_server" {
		var config struct {
			BaseURL string `json:"baseUrl"`
		}
		if err := json.Unmarshal(credential.ProviderConfig, &config); err != nil || strings.TrimSpace(config.BaseURL) == "" {
			return "", "", errors.New("GitLab Server base URL is unavailable")
		}
		base = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	}
	baseURL, err := url.Parse(base)
	repositoryURL, repoErr := url.Parse(credential.RepositoryURL)
	if err != nil || repoErr != nil || baseURL.Scheme != "https" || repositoryURL.Scheme != "https" || !strings.EqualFold(baseURL.Host, repositoryURL.Host) {
		return "", "", errors.New("GitLab CLI host does not match registered repository")
	}
	basePath := strings.Trim(baseURL.Path, "/")
	if basePath != "" && !strings.HasPrefix(strings.Trim(repositoryURL.Path, "/"), basePath+"/") {
		return "", "", errors.New("GitLab CLI subfolder does not match registered repository")
	}
	return baseURL.Host, basePath, nil
}

func environmentMap(environment []string, reserved ...string) map[string]string {
	values := make(map[string]string, len(environment)+4)
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		remove := false
		for _, key := range reserved {
			if name == key || strings.HasPrefix(name, key) {
				remove = true
				break
			}
		}
		if !remove {
			values[name] = value
		}
	}
	return values
}

func prependPath(values map[string]string, directory string) []string {
	runtimePath := values["PATH"]
	if strings.TrimSpace(runtimePath) == "" {
		runtimePath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}
	values["PATH"] = directory + string(os.PathListSeparator) + runtimePath
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}

func writeCLIWrapper(directory, name, script string) error {
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		return err
	}
	return os.Chmod(path, 0o700)
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
