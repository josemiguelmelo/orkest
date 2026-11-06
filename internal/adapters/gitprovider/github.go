package gitprovider

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

// GitHubCLIAuthAdapter scopes gh's token to the registered checkout origin.
type GitHubCLIAuthAdapter struct{}

func (GitHubCLIAuthAdapter) ProviderNames() []string { return []string{"github"} }

func (GitHubCLIAuthAdapter) ConfigureCLI(runtimeRoot string, environment []string, credentials []configmodel.GitCredential) ([]string, error) {
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
