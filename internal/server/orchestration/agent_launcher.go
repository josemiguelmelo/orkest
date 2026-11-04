package orchestration

import (
	"context"
)

// ProcessExecutor hides process creation behind the selected executor implementation.
type ProcessExecutor interface {
	StartAgentctl(context.Context, RunConfig) (Process, error)
}

// AgentProcessController inspects and closes processes started by an executor.
type AgentProcessController interface {
	AgentctlRunning(context.Context, string) (bool, error)
	AgentctlRunningByRunID(context.Context, string) (bool, error)
	TerminateAgentctl(context.Context, string) error
	TerminateAgentctlByRunID(context.Context, string) error
}
