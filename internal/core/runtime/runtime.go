// Package runtime defines the runtimes supported across the platform and agentctl.
package runtime

type Runtime string

const (
	OpenCode   Runtime = "opencode"
	ClaudeCode Runtime = "claude-code"
)

// Definition contains the provider/model fields used by runtime-specific
// definition validation, without importing the agent domain into this package.
type Definition struct {
	ModelProvider string
}

// Provider owns the runtime-specific validation and release metadata.
type Provider interface {
	Runtime() Runtime
	Version() string
	Image() string
	ValidateDefinition(Definition) error
}

// Providers is the canonical catalogue of runtime providers enabled in Orkest.
func Providers() []Provider {
	return []Provider{
		OpenCodeProvider{},
		ClaudeCodeProvider{},
	}
}

// Supported returns the runtime/version map derived from the provider catalogue.
func Supported() map[Runtime]string {
	versions := make(map[Runtime]string)
	for _, provider := range Providers() {
		versions[provider.Runtime()] = provider.Version()
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
