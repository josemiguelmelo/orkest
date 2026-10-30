package runtime

import (
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
)

const ClaudeCodeVersion = "0.99.0"

const ClaudeCodeImage = "orkest/agentctl-claude-code:0.99.0"

// ClaudeCodeProvider defines Claude Code's version and validation rules.
type ClaudeCodeProvider struct{}

func (ClaudeCodeProvider) Runtime() Runtime { return ClaudeCode }

func (ClaudeCodeProvider) Version() string { return ClaudeCodeVersion }

func (ClaudeCodeProvider) Image() string { return ClaudeCodeImage }

func (ClaudeCodeProvider) ValidateDefinition(definition Definition) error {
	if !strings.EqualFold(definition.ModelProvider, "anthropic") {
		return modelerrors.ErrInvalid
	}
	return nil
}

var _ Provider = ClaudeCodeProvider{}
