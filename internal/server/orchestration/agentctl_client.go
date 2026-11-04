package orchestration

import (
	"context"
)

type AgentConnectionClient interface {
	Status(context.Context, string) (RunStatus, error)
	Prompt(context.Context, string, string, string) error
	Connect(context.Context, string, uint64, func(), func(AgentActivityEvent) error) error
}

// AgentctlClient owns request and stream transport to the agentctl process.
type AgentctlClient interface {
	AgentConnectionClient
	WaitHealthy(context.Context, string) error
	Configure(context.Context, string, RunConfig) error
	StartAgent(context.Context, string) error
	RestoreAgent(context.Context, string) error
	CancelPrompt(context.Context, string, string) error
	RespondPermission(context.Context, string, PermissionResponse) error
	Queue(context.Context, string, bool) error
}
