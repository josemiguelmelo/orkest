package opencodetest

import (
	sharedacp "github.com/josemiguelmelo/orkest/internal/agentctl/runtime/acp"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtime/opencode"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtimehost"
)

// NewHost wires the OpenCode profile and ACP client for fixture-backed tests.
func NewHost(binary string, environment runtimehost.Environment) *runtimehost.Host {
	return runtimehost.New(opencode.Profile{Binary: binary}, sharedacp.Factory{}, environment)
}
