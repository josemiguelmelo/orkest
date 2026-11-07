package orchestration

import (
	"context"

	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

type AgentConnectionClient interface {
	Status(context.Context, string) (orchestrationmodel.RunStatus, error)
	Prompt(context.Context, string, string, string) error
	Connect(context.Context, string, uint64, func(), func(orchestrationmodel.AgentActivityEvent) error) error
}

// AgentctlClient owns request and stream transport to the agentctl process.
type AgentctlClient interface {
	AgentConnectionClient
	WaitHealthy(context.Context, string) error
	Configure(context.Context, string, orchestrationmodel.RunConfig) error
	StartAgent(context.Context, string) error
	RestoreAgent(context.Context, string) error
	CancelPrompt(context.Context, string, string) error
	RespondPermission(context.Context, string, orchestrationmodel.PermissionResponse) error
	RespondQuestion(context.Context, string, orchestrationmodel.QuestionResponse) error
	Queue(context.Context, string, bool) error
}
