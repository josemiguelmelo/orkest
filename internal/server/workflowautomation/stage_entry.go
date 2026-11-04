package workflowautomation

import "github.com/josemiguelmelo/orkest/internal/server/workflow"

// PlanTaskEntry resolves the assignment for entering a stage without starting work.
func PlanTaskEntry(definition workflow.Definition, stageID, assigneeID string) (StageTransitionPlan, error) {
	return PlanStageTransition(definition, stageID, stageID, assigneeID, Enter)
}
