package runtime

import (
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
)

// KiroProvider defines Kiro CLI's ACP runtime metadata and model validation.
type KiroProvider struct{}

func (KiroProvider) Runtime() Runtime { return Kiro }

func (KiroProvider) Version() string { return Supported()[Kiro] }

func (p KiroProvider) DefaultImage() string { return "orkest/agentctl-kiro:" + p.Version() }

func (KiroProvider) ValidateDefinition(definition Definition) error {
	if !strings.EqualFold(definition.ModelProvider, "kiro") {
		return modelerrors.ErrInvalid
	}
	return nil
}

var _ Provider = KiroProvider{}
