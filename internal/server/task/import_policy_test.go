package task

import (
	"testing"

	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
)

func TestImportedTaskLifecycleUsesConfiguredStages(t *testing.T) {
	d := workflowmodel.DefaultDefinition("Imported")
	d.InitialStageID = "in_review"
	d.Stages[2].ResponsibleAgentID = "reviewer"
	service := &TaskService{}
	for _, tc := range []struct {
		name           string
		exists, closed bool
		current, want  string
		entry          bool
	}{
		{"new task", false, false, "", "in_review", true},
		{"repeat import", true, false, "in_review", "in_review", false},
		{"source closes", true, true, "in_review", "done", true},
		{"closed source stays closed", true, true, "done", "done", false},
		{"open source retains local stage", true, false, "done", "done", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			effects, err := service.PlanImportedTask(ImportedTaskSnapshot{Workflow: d, SourceID: "source", Title: "Task", Exists: tc.exists, SourceClosed: tc.closed, StageID: tc.current, AssigneeAgentID: "current"})
			if err != nil || effects.StageID != tc.want || effects.EmitStageEntry != tc.entry {
				t.Fatalf("effects=%+v, error=%v", effects, err)
			}
			if tc.name == "new task" && effects.AssigneeAgentID != "reviewer" {
				t.Fatal("initial assignment missing", effects)
			}
		})
	}
}
