package orchestration

import (
	"context"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentrun "github.com/josemiguelmelo/orkest/internal/agentctl/run"
)

type AgentConnectionClient interface {
	Status(context.Context, string) (agentrun.RunStatus, error)
	Prompt(context.Context, string, string, string) error
	Connect(context.Context, string, uint64, func(), func(interaction.Event) error) error
}

// AgentctlClient owns request and stream transport to the agentctl process.
type AgentctlClient interface {
	AgentConnectionClient
	WaitHealthy(context.Context, string) error
	Configure(context.Context, string, runconfig.RunConfig) error
	StartAgent(context.Context, string) error
	RestoreAgent(context.Context, string) error
	CancelPrompt(context.Context, string, string) error
	RespondPermission(context.Context, string, interaction.PermissionResponse) error
	Queue(context.Context, string, bool) error
}
