package orchestration

import (
	"context"
	"errors"
)

// AgentLauncher is deliberately limited to creating the agentctl process.
type AgentLauncher interface {
	LaunchAgentctl(context.Context, RunConfig) (Process, error)
}

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

type ExecutorAgentLauncher struct{ executor ProcessExecutor }

func NewAgentLauncher(executor ProcessExecutor) (*ExecutorAgentLauncher, error) {
	if executor == nil {
		return nil, errors.New("process executor is required")
	}
	return &ExecutorAgentLauncher{executor: executor}, nil
}

func (l *ExecutorAgentLauncher) LaunchAgentctl(ctx context.Context, config RunConfig) (Process, error) {
	return l.executor.StartAgentctl(ctx, config)
}
