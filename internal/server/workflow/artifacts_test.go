package workflow

import (
	"testing"

	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
)

func TestArtifactBindingValidation(t *testing.T) {
	definition := workflowmodel.DefaultDefinition("Artifacts")
	definition.ID = "workflow"
	definition.Stages[0].ArtifactOutputs = []workflowmodel.ArtifactOutput{{Name: "implementation_plan", Kind: "plan"}}
	definition.Stages[1].ArtifactInputs = []workflowmodel.ArtifactInput{{Name: "plan", SourceStageID: "todo", OutputName: "implementation_plan", Required: true}}
	if err := definition.Validate(); err != nil {
		t.Fatal(err)
	}
	definition.Stages[1].ArtifactInputs[0].OutputName = "missing"
	if err := definition.Validate(); err == nil {
		t.Fatal("missing output accepted")
	}
	definition.Stages[1].ArtifactInputs[0].OutputName = "implementation_plan"
	definition.Stages[0].ArtifactOutputs = append(definition.Stages[0].ArtifactOutputs, definition.Stages[0].ArtifactOutputs[0])
	if err := definition.Validate(); err == nil {
		t.Fatal("ambiguous output accepted")
	}
}
