package runtime

import (
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
)

// ClaudeCodeProvider defines Claude Code's version and validation rules.
type ClaudeCodeProvider struct{}

func (ClaudeCodeProvider) Runtime() Runtime { return ClaudeCode }

func (ClaudeCodeProvider) Version() string { return Supported()[ClaudeCode] }

func (p ClaudeCodeProvider) DefaultImage() string {
	return "orkest/agentctl-claude-code:" + p.Version()
}

func (ClaudeCodeProvider) ValidateDefinition(definition Definition) error {
	if !strings.EqualFold(definition.ModelProvider, "anthropic") {
		return modelerrors.ErrInvalid
	}
	return nil
}

func (ClaudeCodeProvider) ConfigurationSchema() []ConfigurationOption {
	return []ConfigurationOption{{Name: "ANTHROPIC_API_KEY", Description: "Anthropic API key", Required: false, Sensitive: true}}
}

var _ Provider = ClaudeCodeProvider{}
