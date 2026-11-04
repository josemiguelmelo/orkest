package orchestration

import "github.com/josemiguelmelo/orkest/internal/server/workflow"

type LaunchStatusEffects struct {
	RunID   string
	Status  LaunchStatus
	Outcome *workflow.EventIntent
}

// PlanLaunchStatus emits an outcome only after execution termination is confirmed.
func PlanLaunchStatus(runID string, status LaunchStatus) LaunchStatusEffects {
	effects := LaunchStatusEffects{RunID: runID, Status: status}
	switch status {
	case LaunchCompleted, LaunchFailed:
		kind := workflow.RunFailed
		if status == LaunchCompleted {
			kind = workflow.RunSucceeded
		}
		effects.Outcome = &workflow.EventIntent{ID: runID + ":outcome", Type: kind, RunID: runID, CausationID: runID, CorrelationID: runID}
	}
	return effects
}
