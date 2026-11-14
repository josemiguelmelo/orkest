package artifactinput

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
	inputs, err := Resolve(definition, stage, available)
	if err != nil || len(inputs["plan"]) != 2 || inputs["plan"][0].ID != "first" {
		t.Fatalf("handoff: %+v, %v", inputs, err)
	}
	available[1].Title = "changed"
	if inputs["plan"][0].Title != "" {
		t.Fatal("input changed after resolution")
	}
	if _, err := Resolve(definition, stage, nil); err == nil {
		t.Fatal("missing required input accepted")
	}
	stage.ArtifactInputs[0].Required = false
	if _, err := Resolve(definition, stage, nil); err != nil {
		t.Fatal(err)
	}
}

func TestResolveInputUsesDeclaredKind(t *testing.T) {
	binding := workflowmodel.ArtifactInput{Name: "pr", SourceStageID: "build", OutputName: "pr", Required: true}
	definition := workflowmodel.Definition{Stages: []workflowmodel.Stage{{ID: "build", ArtifactOutputs: []workflowmodel.ArtifactOutput{{Name: "pr", Kind: "code_review"}}}}}
	available := []artifactmodel.Revision{
		{ID: "previous-kind", Kind: "pull_request", StageID: "build", OutputName: "pr", StageVersion: 9},
	}
	if _, err := ResolveInput(definition, binding, available); err == nil {
		t.Fatal("previous output kind satisfies required input")
	}
	binding.Required = false
	if revisions, err := ResolveInput(definition, binding, available); err != nil || len(revisions) != 0 {
		t.Fatalf("optional input: %v, %v", revisions, err)
	}
	available = append(available,
		artifactmodel.Revision{ID: "old", Kind: "code_review", StageID: "build", OutputName: "pr", StageVersion: 2},
		artifactmodel.Revision{ID: "current", Kind: "code_review", StageID: "build", OutputName: "pr", StageVersion: 3},
	)
	stage := workflowmodel.Stage{ArtifactInputs: []workflowmodel.ArtifactInput{binding}}
	inputs, err := Resolve(definition, stage, available)
	if err != nil || len(inputs["pr"]) != 1 || inputs["pr"][0].ID != "current" {
		t.Fatalf("declared kind selection: %v, %v", inputs, err)
	}
}

func TestResolveInputRejectsMissingDeclaration(t *testing.T) {
	binding := workflowmodel.ArtifactInput{Name: "pr", SourceStageID: "build", OutputName: "pr"}
	definition := workflowmodel.Definition{}
	if _, err := ResolveInput(definition, binding, nil); err == nil {
		t.Fatal("missing source accepted")
	}
	definition.Stages = []workflowmodel.Stage{{ID: "build"}}
	if _, err := ResolveInput(definition, binding, nil); err == nil {
		t.Fatal("missing output accepted")
	}
}
