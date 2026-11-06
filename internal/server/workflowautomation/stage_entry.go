package workflowautomation

import (
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
)

// PlanTaskEntry resolves the assignment for entering a stage without starting work.
func PlanTaskEntry(definition workflowmodel.Definition, stageID, assigneeID string) (StageTransitionPlan, error) {
	return PlanStageTransition(definition, stageID, stageID, assigneeID, Enter)
}
