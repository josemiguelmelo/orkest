package orchestration

import (
	"context"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

// AgentctlClient owns request and stream transport to the agentctl process.
type AgentctlClient interface {
	WaitHealthy(context.Context, string) error
	Configure(context.Context, string, agentctl.RunConfig) error
	StartAgent(context.Context, string) error
	Prompt(context.Context, string, string, string) error
	Connect(context.Context, string, uint64, func(), func(agentctl.Event) error) error
}
