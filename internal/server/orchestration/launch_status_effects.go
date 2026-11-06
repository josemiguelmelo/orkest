package orchestration

import (
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	"github.com/josemiguelmelo/orkest/internal/server/workflow"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
)

type LaunchStatusEffects struct {
	RunID   string
	Status  orchestrationmodel.LaunchStatus
	Outcome *workflow.EventIntent
}

// PlanLaunchStatus emits an outcome only after execution termination is confirmed.
func PlanLaunchStatus(runID string, status orchestrationmodel.LaunchStatus) LaunchStatusEffects {
	effects := LaunchStatusEffects{RunID: runID, Status: status}
	switch status {
	case orchestrationmodel.LaunchCompleted, orchestrationmodel.LaunchFailed:
		kind := workflowmodel.RunFailed
		if status == orchestrationmodel.LaunchCompleted {
			kind = workflowmodel.RunSucceeded
		}
		effects.Outcome = &workflow.EventIntent{ID: runID + ":outcome", Type: kind, RunID: runID, CausationID: runID, CorrelationID: runID}
	}
	return effects
}
