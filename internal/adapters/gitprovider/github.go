package gitprovider

import (
	"errors"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

// GitHubCLIAuthAdapter exports the workspace integration token for gh.
type GitHubCLIAuthAdapter struct{}

func (GitHubCLIAuthAdapter) ProviderNames() []string { return []string{"github"} }

func (GitHubCLIAuthAdapter) ConfigureCLI(_ string, environment []string, credentials []configmodel.GitCredential) ([]string, error) {
	if len(credentials) == 0 {
		return environment, nil
	}
	token := credentials[0].Token
	for _, credential := range credentials[1:] {
		if credential.Token != token {
			return nil, &agentruntime.LaunchError{Message: "configure GitHub CLI authentication failed", Cause: errors.New("conflicting GitHub credentials")}
		}
	}
	values := environmentMap(environment, "GH_TOKEN", "GITHUB_TOKEN", "ORKEST_GH_")
	values["GH_TOKEN"] = token
	values["GH_PROMPT_DISABLED"] = "1"
	result := make([]string, 0, len(values))
	for name, value := range values {
		result = append(result, name+"="+value)
	}
	return result, nil
}
