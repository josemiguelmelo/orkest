package runtime

import (
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
)

// CodexProvider defines the Codex runtime's version and validation rules.
type CodexProvider struct{}

func (CodexProvider) Runtime() Runtime { return Codex }

func (CodexProvider) Version() string { return Supported()[Codex] }

func (p CodexProvider) DefaultImage() string { return "orkest/agentctl-codex:" + p.Version() }

func (CodexProvider) ValidateDefinition(definition Definition) error {
	if !strings.EqualFold(definition.ModelProvider, "openai") {
		return modelerrors.ErrInvalid
	}
	return nil
}

var _ Provider = CodexProvider{}
