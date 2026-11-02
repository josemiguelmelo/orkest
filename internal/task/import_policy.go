package task

import (
	"fmt"

	"github.com/josemiguelmelo/orkest/internal/workflow"
)

type ImportedTaskSnapshot struct {
	Workflow                 workflow.Definition
	SourceID, Title          string
	Exists                   bool
	SourceClosed             bool
	StageID, AssigneeAgentID string
}

type ImportedTaskEffects struct {
	StageID, AssigneeAgentID string
	EmitStageEntry           bool
	WorkState                WorkState
}

// PlanImportedTask runs against the workflow and existing task locked by the import transaction.
func (s *TaskService) PlanImportedTask(snapshot ImportedTaskSnapshot) (ImportedTaskEffects, error) {
	if snapshot.SourceID == "" || snapshot.Title == "" {
		return ImportedTaskEffects{}, fmt.Errorf("source task is missing identity or title")
	}
	stageID := snapshot.StageID
	if !snapshot.Exists {
		stageID = snapshot.Workflow.InitialStageID
	}
	if snapshot.SourceClosed {
		stageID = snapshot.Workflow.ClosedStageID
	}
	effects := ImportedTaskEffects{StageID: stageID, AssigneeAgentID: snapshot.AssigneeAgentID}
	if snapshot.Exists && stageID == snapshot.StageID {
		return effects, nil
	}
	stage, ok := snapshot.Workflow.Stage(stageID)
	if !ok {
		effects.WorkState = WorkStateFailed
		return effects, nil
	}
	if stage.ResponsibleAgentID != "" {
		effects.AssigneeAgentID = stage.ResponsibleAgentID
	}
	effects.EmitStageEntry = true

	return effects, nil
}
