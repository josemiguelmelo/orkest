package orchestration

import (
	"testing"

	artifactmodel "github.com/josemiguelmelo/orkest/internal/server/artifact/model"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
)

func TestResolveArtifactHandoff(t *testing.T) {
	stage := workflowmodel.Stage{ArtifactInputs: []workflowmodel.ArtifactInput{{Name: "plan", SourceStageID: "planning", OutputName: "implementation_plan", Required: true}}}
	definition := workflowmodel.Definition{Stages: []workflowmodel.Stage{{ID: "planning", ArtifactOutputs: []workflowmodel.ArtifactOutput{{Name: "implementation_plan", Kind: "plan"}}}}}
	available := []artifactmodel.Revision{
		{ID: "old", Kind: "plan", StageID: "planning", OutputName: "implementation_plan", StageVersion: 1},
		{ID: "first", Kind: "plan", StageID: "planning", OutputName: "implementation_plan", StageVersion: 3},
		{ID: "second", Kind: "plan", StageID: "planning", OutputName: "implementation_plan", StageVersion: 3},
		{ID: "unrelated", StageID: "review", OutputName: "implementation_plan", StageVersion: 4},
	}
	inputs, err := resolveArtifactInputs(definition, stage, available)
	if err != nil || len(inputs["plan"]) != 2 || inputs["plan"][0].ID != "first" {
		t.Fatalf("handoff: %+v, %v", inputs, err)
	}
	available[1].Title = "changed"
	if inputs["plan"][0].Title != "" {
		t.Fatal("input changed after resolution")
	}
	if _, err := resolveArtifactInputs(definition, stage, nil); err == nil {
		t.Fatal("missing required input accepted")
	}
	stage.ArtifactInputs[0].Required = false
	if _, err := resolveArtifactInputs(definition, stage, nil); err != nil {
		t.Fatal(err)
	}
}
