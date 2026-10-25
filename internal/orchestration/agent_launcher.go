package orchestration

import (
	"context"
	"errors"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

// AgentLauncher is deliberately limited to creating the agentctl process.
type AgentLauncher interface {
	LaunchAgentctl(context.Context, agentctl.RunConfig) (Process, error)
}

// ProcessExecutor hides process creation behind the selected executor implementation.
type ProcessExecutor interface {
	StartAgentctl(context.Context, agentctl.RunConfig) (Process, error)
}

type ExecutorAgentLauncher struct{ executor ProcessExecutor }

func NewAgentLauncher(executor ProcessExecutor) (*ExecutorAgentLauncher, error) {
	if executor == nil {
		return nil, errors.New("process executor is required")
	}
	return &ExecutorAgentLauncher{executor: executor}, nil
}

func (l *ExecutorAgentLauncher) LaunchAgentctl(ctx context.Context, config agentctl.RunConfig) (Process, error) {
	return l.executor.StartAgentctl(ctx, config)
}
