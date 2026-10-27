package run

import (
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

// ExecutionState describes execution progress independently of process liveness.
type ExecutionState string

const (
	StateStarting          ExecutionState = "starting"
	StateReady             ExecutionState = "ready"
	StateRunning           ExecutionState = "running"
	StateWaitingPermission ExecutionState = "waiting_for_permission"
	StateFailed            ExecutionState = "failed"
	StateInterrupted       ExecutionState = "interrupted"
	StateExited            ExecutionState = "exited"
)

// observedExecutionState combines conversation progress with confirmed process
// liveness for status reporting. A live process never clears an interruption;
// a stopped process only replaces ordinary ready/running execution states.
func observedExecutionState(progress ExecutionState, session agentruntime.RuntimeSession) (ExecutionState, bool) {
	if session == nil {
		return progress, false
	}
	select {
	case <-session.Done():
		if progress == StateInterrupted || progress == StateFailed {
			return progress, false
		}
		if session.Err() != nil {
			return StateFailed, false
		}
		return StateExited, false
	default:
		return progress, true
	}
}
