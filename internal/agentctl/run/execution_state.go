package run

import (
	runmodel "github.com/josemiguelmelo/orkest/internal/agentctl/run/model"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

// observedExecutionState combines conversation progress with confirmed process
// liveness for status reporting. A live process never clears an interruption;
// a stopped process only replaces ordinary ready/running execution states.
func observedExecutionState(progress runmodel.ExecutionState, session agentruntime.RuntimeSession) (runmodel.ExecutionState, bool) {
	if session == nil {
		return progress, false
	}
	select {
	case <-session.Done():
		if progress == runmodel.StateInterrupted || progress == runmodel.StateFailed {
			return progress, false
		}
		if session.Err() != nil {
			return runmodel.StateFailed, false
		}
		return runmodel.StateExited, false
	default:
		return progress, true
	}
}
