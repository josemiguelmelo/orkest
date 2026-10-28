package gitprovider

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

// Preparation contains the runtime paths, environment, and repository credentials
// needed to prepare Git and its provider CLIs.
type Preparation struct {
	RuntimeRoot string
	WorkDir     string
	Environment []string
	Credentials []runconfig.GitCredential
}

// PrepareRuntime configures repository identity, Git transport credentials, and
// matching provider CLIs. It returns the environment the runtime should inherit.
func PrepareRuntime(ctx context.Context, preparation Preparation, adapters []Adapter) ([]string, error) {
	if len(preparation.Credentials) == 0 {
		return preparation.Environment, nil
	}
	if err := configureGit(ctx, preparation.WorkDir, preparation.Credentials); err != nil {
		return nil, err
	}
	environment := withGitCredentials(preparation.Environment, preparation.Credentials)
	return configureProviderCLIs(preparation.RuntimeRoot, environment, preparation.Credentials, adapters)
}

func configureGit(ctx context.Context, workDir string, credentials []runconfig.GitCredential) error {
	for _, credential := range credentials {
		if filepath.IsAbs(credential.RepositoryPath) || filepath.Clean(credential.RepositoryPath) != credential.RepositoryPath || strings.HasPrefix(credential.RepositoryPath, "..") {
			return &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "configure Git identity failed", Cause: fmt.Errorf("invalid repository path")}
		}
		path := filepath.Join(workDir, credential.RepositoryPath)
		for _, setting := range [][2]string{{"user.name", "Orkest Agent"}, {"user.email", "agent@orkest.local"}} {
			cmd := exec.CommandContext(ctx, "git", "-C", path, "config", "--local", setting[0], setting[1])
			cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
			if err := cmd.Run(); err != nil {
				return &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "configure Git identity failed", Cause: err}
			}
		}
	}
	return nil
}

func withGitCredentials(environment []string, credentials []runconfig.GitCredential) []string {
	values := make(map[string]string, len(environment)+1+len(credentials)*2)
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name != "GIT_CONFIG_COUNT" && !strings.HasPrefix(name, "GIT_CONFIG_KEY_") && !strings.HasPrefix(name, "GIT_CONFIG_VALUE_") {
			values[name] = value
		}
	}
	values["GIT_CONFIG_COUNT"] = strconv.Itoa(len(credentials))
	for index, credential := range credentials {
		auth := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + credential.Token))
		values["GIT_CONFIG_KEY_"+strconv.Itoa(index)] = "http." + credential.RepositoryURL + ".extraheader"
		values["GIT_CONFIG_VALUE_"+strconv.Itoa(index)] = "AUTHORIZATION: basic " + auth
	}
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}
