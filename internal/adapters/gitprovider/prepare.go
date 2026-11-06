package gitprovider

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

// RuntimePreparation contains the runtime paths, environment, and repository credentials
// needed to prepare Git and its provider CLIs.
type RuntimePreparation struct {
	RuntimeRoot string
	WorkDir     string
	Environment []string
	Credentials []configmodel.GitCredential
}

// PrepareRuntime configures repository identity, Git transport credentials, and
// matching provider CLIs. It returns the environment the runtime should inherit.
func PrepareRuntime(ctx context.Context, preparation RuntimePreparation, adapters []CLIAuthAdapter) ([]string, error) {
	if len(preparation.Credentials) == 0 {
		return preparation.Environment, nil
	}
	if err := configureRepositoryIdentity(ctx, preparation.WorkDir, preparation.Credentials); err != nil {
		return nil, err
	}
	environment := withGitHTTPAuth(preparation.Environment, preparation.Credentials)
	return configureCLIs(preparation.RuntimeRoot, environment, preparation.Credentials, adapters)
}

func configureRepositoryIdentity(ctx context.Context, workDir string, credentials []configmodel.GitCredential) error {
	for _, credential := range credentials {
		if filepath.IsAbs(credential.RepositoryPath) || filepath.Clean(credential.RepositoryPath) != credential.RepositoryPath || strings.HasPrefix(credential.RepositoryPath, "..") {
			return &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "configure Git identity failed", Cause: fmt.Errorf("invalid repository path")}
		}
		path := filepath.Join(workDir, credential.RepositoryPath)
		for _, setting := range [][2]string{{"user.name", "Orkest Agent"}, {"user.email", "agent@orkest.local"}} {
			cmd := exec.CommandContext(ctx, "git", "-C", path, "config", "--local", setting[0], setting[1])
			var stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = io.Discard, &stderr
			if err := cmd.Run(); err != nil {
				cause := fmt.Errorf("git config %s failed: %w", setting[0], err)
				if output := strings.TrimSpace(stderr.String()); output != "" {
					cause = fmt.Errorf("%w: %s", cause, output)
				}
				return &agentruntime.RuntimeError{Kind: agentruntime.RuntimeFailure, Message: "configure Git identity failed", Cause: cause}
			}
		}
	}
	return nil
}

func withGitHTTPAuth(environment []string, credentials []configmodel.GitCredential) []string {
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
