package task

import (
	"fmt"

	"github.com/josemiguelmelo/orkest/internal/server/workflow"
)

type ImportedTaskSnapshot struct {
	Workflow                 workflow.Definition
	TaskID                   string
	StageVersion             int64
	SourceID, Title          string
	Exists                   bool
	SourceClosed             bool
	StageID, AssigneeAgentID string
}

type ImportedTaskEffects struct {
	StageID, AssigneeAgentID string
	EmitStageEntry           bool
	WorkState                WorkState
	Event                    *workflow.EventIntent
	StageVersion             int64
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
	version := snapshot.StageVersion
	if !snapshot.Exists {
		version = 1
	} else if snapshot.StageID != stageID {
		version++
	}
	effects.StageVersion = version
	event := workflow.StageEvent(snapshot.TaskID, version, workflow.StageEntered, "", "", "")
	effects.Event = &event

	return effects, nil
}
