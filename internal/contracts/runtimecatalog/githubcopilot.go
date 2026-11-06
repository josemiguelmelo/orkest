package runtime

import (
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
)

// GitHubCopilotProvider defines GitHub Copilot CLI's ACP runtime metadata.
type GitHubCopilotProvider struct{}

func (GitHubCopilotProvider) Runtime() Runtime { return GitHubCopilot }

func (GitHubCopilotProvider) Version() string { return Supported()[GitHubCopilot] }

func (p GitHubCopilotProvider) DefaultImage() string {
	return "orkest/agentctl-github-copilot:" + p.Version()
}

func (GitHubCopilotProvider) ValidateDefinition(definition Definition) error {
	if !strings.EqualFold(definition.ModelProvider, "github-copilot") {
		return modelerrors.ErrInvalid
	}
	return nil
}

func (GitHubCopilotProvider) ConfigurationSchema() []ConfigurationOption {
	return []ConfigurationOption{
		{Name: "COPILOT_GITHUB_TOKEN", Description: "GitHub Copilot authentication token", Sensitive: true},
		{Name: "GH_TOKEN", Description: "GitHub CLI authentication token", Sensitive: true},
		{Name: "GITHUB_TOKEN", Description: "GitHub authentication token", Sensitive: true},
	}
}

var _ Provider = GitHubCopilotProvider{}
