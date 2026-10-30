package runtime

const OpenCodeVersion = "opencode v2.1.4"

const OpenCodeImage = "orkest/agentctl-opencode:2.1.4"

// OpenCodeProvider defines the OpenCode runtime's version and validation rules.
type OpenCodeProvider struct{}

func (OpenCodeProvider) Runtime() Runtime { return OpenCode }

func (OpenCodeProvider) Version() string { return OpenCodeVersion }

func (OpenCodeProvider) Image() string { return OpenCodeImage }

func (OpenCodeProvider) ValidateDefinition(Definition) error { return nil }

var _ Provider = OpenCodeProvider{}
