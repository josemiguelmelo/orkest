// Package gitprovider prepares Git credentials and provider CLIs for a runtime.
package gitprovider

import (
	"github.com/josemiguelmelo/orkest/internal/agentctl/config"
)

// Adapter configures one or more provider CLIs for a runtime environment.
type Adapter interface {
	ProviderNames() []string
	Configure(string, []string, []config.GitCredential) ([]string, error)
}

// DefaultAdapters returns the provider CLI strategies shipped with agentctl.
func DefaultAdapters() []Adapter {
	return []Adapter{GitHubAdapter{}, GitLabAdapter{}}
}

func configureProviderCLIs(runtimeRoot string, environment []string, credentials []config.GitCredential, adapters []Adapter) ([]string, error) {
	for _, adapter := range adapters {
		matched := credentialsForProviders(credentials, adapter.ProviderNames())
		if len(matched) == 0 {
			continue
		}
		var err error
		environment, err = adapter.Configure(runtimeRoot, environment, matched)
		if err != nil {
			return nil, err
		}
	}
	return environment, nil
}

func credentialsForProviders(credentials []config.GitCredential, providers []string) []config.GitCredential {
	allowed := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		allowed[provider] = struct{}{}
	}
	matched := make([]config.GitCredential, 0, len(credentials))
	for _, credential := range credentials {
		if _, ok := allowed[credential.ProviderName]; ok {
			matched = append(matched, credential)
		}
	}
	return matched
}
