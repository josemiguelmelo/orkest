package claudecodetest

import (
	sharedacp "github.com/josemiguelmelo/orkest/internal/agentctl/runtime/acp"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtime/claudecode"
	"github.com/josemiguelmelo/orkest/internal/agentctl/runtimehost"
)

// NewHost wires the Claude Code profile and ACP client for fixture-backed tests.
func NewHost(binary string, environment runtimehost.Environment) *runtimehost.Host {
	return runtimehost.New(claudecode.Profile{Binary: binary}, sharedacp.Factory{}, environment)
}
