package orchestration

import (
	"context"

	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

// AgentctlStarter starts an independently owned agentctl worker.
type AgentctlStarter interface {
	StartAgentctl(context.Context, orchestrationmodel.RunConfig) (orchestrationmodel.Process, error)
}

// AgentProcessController inspects and closes processes started by an executor.
type AgentProcessController interface {
	AgentctlRunning(context.Context, string) (bool, error)
	AgentctlRunningByRunID(context.Context, string) (bool, error)
	TerminateAgentctl(context.Context, string) error
	TerminateAgentctlByRunID(context.Context, string) error
}
