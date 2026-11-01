package runtime

import "strings"

// OpenCodeProvider defines the OpenCode runtime's version and validation rules.
type OpenCodeProvider struct{}

func (OpenCodeProvider) Runtime() Runtime { return OpenCode }

func (OpenCodeProvider) Version() string { return Supported()[OpenCode] }

func (p OpenCodeProvider) DefaultImage() string {
	return "orkest/agentctl-opencode:" + strings.TrimPrefix(p.Version(), "opencode v")
}

func (OpenCodeProvider) ValidateDefinition(Definition) error { return nil }

func (OpenCodeProvider) ConfigurationSchema() []ConfigurationOption {
	return []ConfigurationOption{
		{Name: "OPENAI_API_KEY", Description: "OpenAI API key", Sensitive: true},
		{Name: "ANTHROPIC_API_KEY", Description: "Anthropic API key", Sensitive: true},
		{Name: "OPENROUTER_API_KEY", Description: "OpenRouter API key", Sensitive: true},
	}
}

var _ Provider = OpenCodeProvider{}
