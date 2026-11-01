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

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

// GitHubCLIAuthAdapter scopes gh's token to the registered checkout origin.
type GitHubCLIAuthAdapter struct{}

func (GitHubCLIAuthAdapter) ProviderNames() []string { return []string{"github"} }

func (GitHubCLIAuthAdapter) ConfigureCLI(runtimeRoot string, environment []string, credentials []runconfig.GitCredential) ([]string, error) {
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
	// gh accepts --repo/-R for operations launched from a task's common work
	// directory. Resolve that explicit target first; otherwise use the checkout
	// origin as before. Keep the original arguments intact for the real gh.
	script.WriteString("repo=\nrepo_next=0\nfor arg in \"$@\"; do\n  if [ \"$repo_next\" = 1 ]; then repo=$arg; repo_next=0; continue; fi\n  case \"$arg\" in\n    -R|--repo) repo_next=1 ;;\n    --repo=*) repo=${arg#*=} ;;\n  esac\ndone\n")
	script.WriteString("if [ -z \"$repo\" ]; then\n  repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || { echo 'gh requires a registered repository checkout or --repo target' >&2; exit 1; }\n  repo=$(git -C \"$repo_root\" remote get-url origin 2>/dev/null) || { echo 'gh could not read this checkout origin' >&2; exit 1; }\nfi\n")
	script.WriteString("case \"$repo\" in\n")
	for index, credential := range credentials {
		repository, err := url.Parse(credential.RepositoryURL)
		if err != nil || repository.Scheme != "https" || !strings.EqualFold(repository.Hostname(), "github.com") {
			return nil, &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "configure GitHub CLI authentication failed", Cause: errors.New("GitHub CLI repository host is invalid")}
		}
		canonical := strings.TrimSuffix(credential.RepositoryURL, ".git")
		repositoryPath := strings.TrimPrefix(canonical, "https://github.com/")
		fmt.Fprintf(&script, "  %s|%s|%s) GH_TOKEN=\"${ORKEST_GH_TOKEN_%d:?GitHub credential unavailable}\"; export GH_TOKEN ;;\n", shellSingleQuote(canonical), shellSingleQuote(canonical+".git"), shellSingleQuote(repositoryPath), index)
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
