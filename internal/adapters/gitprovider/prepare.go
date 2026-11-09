package gitprovider

import (
	"context"
	"encoding/base64"
	"strconv"
	"strings"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
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
func PrepareRuntime(_ context.Context, preparation RuntimePreparation, adapters []CLIAuthAdapter) ([]string, error) {
	environment := withGitHTTPAuth(preparation.Environment, preparation.Credentials)
	if len(preparation.Credentials) == 0 {
		return environment, nil
	}
	return configureCLIs(preparation.RuntimeRoot, environment, preparation.Credentials, adapters)
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
	values["GIT_AUTHOR_NAME"] = values["ORKEST_GIT_AUTHOR_NAME"]
	values["GIT_AUTHOR_EMAIL"] = values["ORKEST_GIT_AUTHOR_EMAIL"]
	values["GIT_COMMITTER_NAME"] = values["ORKEST_GIT_AUTHOR_NAME"]
	values["GIT_COMMITTER_EMAIL"] = values["ORKEST_GIT_AUTHOR_EMAIL"]
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
