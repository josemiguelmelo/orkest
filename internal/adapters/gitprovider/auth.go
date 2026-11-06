// Package gitprovider prepares Git credentials and provider CLIs for a runtime.
package gitprovider

import (
	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
)

// CLIAuthAdapter configures a provider CLI for credentials in the runtime.
type CLIAuthAdapter interface {
	ProviderNames() []string
	ConfigureCLI(runtimeRoot string, environment []string, credentials []configmodel.GitCredential) ([]string, error)
}

// DefaultCLIAuthAdapters returns the CLI auth strategies shipped with agentctl.
func DefaultCLIAuthAdapters() []CLIAuthAdapter {
	return []CLIAuthAdapter{GitHubCLIAuthAdapter{}, GitLabCLIAuthAdapter{}}
}

func configureCLIs(runtimeRoot string, environment []string, credentials []configmodel.GitCredential, adapters []CLIAuthAdapter) ([]string, error) {
	for _, adapter := range adapters {
		matched := credentialsFor(credentials, adapter.ProviderNames())
		if len(matched) == 0 {
			continue
		}
		var err error
		environment, err = adapter.ConfigureCLI(runtimeRoot, environment, matched)
		if err != nil {
			return nil, err
		}
	}
	return environment, nil
}

func credentialsFor(credentials []configmodel.GitCredential, providers []string) []configmodel.GitCredential {
	allowed := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		allowed[provider] = struct{}{}
	}
	matched := make([]configmodel.GitCredential, 0, len(credentials))
	for _, credential := range credentials {
		if _, ok := allowed[credential.ProviderName]; ok {
			matched = append(matched, credential)
		}
	}
	return matched
}
