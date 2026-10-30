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

var _ Provider = OpenCodeProvider{}
