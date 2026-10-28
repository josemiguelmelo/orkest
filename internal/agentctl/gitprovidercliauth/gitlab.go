package gitprovidercliauth

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

// GitLabAdapter configures glab for GitLab Cloud and self-managed installations.
type GitLabAdapter struct{}

func (GitLabAdapter) ProviderNames() []string { return []string{"gitlab_cloud", "gitlab_server"} }

func (GitLabAdapter) Configure(runtimeRoot string, environment []string, credentials []runconfig.GitCredential) ([]string, error) {
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
