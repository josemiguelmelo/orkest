// Package runtime defines the runtimes supported across the platform and agentctl.
package runtime

import (
	_ "embed"
	"encoding/json"
)

//go:embed versions.json
var versionInventory []byte

type Runtime string

const (
	OpenCode      Runtime = "opencode"
	ClaudeCode    Runtime = "claude-code"
	Codex         Runtime = "codex"
	GitHubCopilot Runtime = "github-copilot"
	Kiro          Runtime = "kiro"
)

// Definition contains the provider/model fields used by runtime-specific
// definition validation, without importing the agent domain into this package.
type Definition struct {
	ModelProvider string
}

// ConfigurationOption describes one environment variable accepted by a runtime.
// Values marked sensitive are write-only in the platform API.
type ConfigurationOption struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Sensitive   bool   `json:"sensitive"`
}

// Provider owns the runtime-specific validation and release metadata.
type Provider interface {
	Runtime() Runtime
	Version() string
	DefaultImage() string
	ValidateDefinition(Definition) error
	ConfigurationSchema() []ConfigurationOption
}

// Providers is the canonical catalogue of runtime providers enabled in Orkest.
func Providers() []Provider {
	return []Provider{
		OpenCodeProvider{},
		ClaudeCodeProvider{},
		CodexProvider{},
		GitHubCopilotProvider{},
		KiroProvider{},
	}
}

// Supported returns a copy of the runtime/version inventory shared with CI.
func Supported() map[Runtime]string {
	var versions map[Runtime]string
	if err := json.Unmarshal(versionInventory, &versions); err != nil {
		panic(err)
	}
	return versions
}

// ProviderFor returns the canonical provider registered for runtime.
func ProviderFor(runtime Runtime) (Provider, bool) {
	for _, provider := range Providers() {
		if provider.Runtime() == runtime {
			return provider, true
		}
	}
	return nil, false
}

// Available reports whether the runtime is implemented by both the platform and agentctl.
func (r Runtime) Available() bool {
	_, ok := Supported()[r]
	return ok
}
