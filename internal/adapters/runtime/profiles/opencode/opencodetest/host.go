package opencodetest

import (
	"github.com/josemiguelmelo/orkest/internal/adapters/gitprovider"
	sharedacp "github.com/josemiguelmelo/orkest/internal/adapters/runtime/acp"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/profiles/opencode"
	"github.com/josemiguelmelo/orkest/internal/adapters/runtime/runtimehost"
)

// NewHost wires the OpenCode profile and ACP client for fixture-backed tests.
func NewHost(binary string, environment runtimehost.Environment) *runtimehost.Host {
	return runtimehost.New(opencode.Profile{Binary: binary}, sharedacp.Factory{}, environment, gitprovider.DefaultCLIAuthAdapters()...)
}
