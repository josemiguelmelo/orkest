// Package artifactinput resolves workflow artifact bindings without persistence or I/O.
package artifactinput

import (
	"fmt"

	artifactmodel "github.com/josemiguelmelo/orkest/internal/server/artifact/model"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
)

// Resolve selects revisions for every input, enforcing required bindings.
func Resolve(definition workflowmodel.Definition, stage workflowmodel.Stage, available []artifactmodel.Revision) (map[string][]artifactmodel.Revision, error) {
	inputs := make(map[string][]artifactmodel.Revision)
	for _, binding := range stage.ArtifactInputs {
		revisions, err := ResolveInput(definition, binding, available)
		if err != nil {
			return nil, err
		}
		inputs[binding.Name] = revisions
	}
	return inputs, nil
}

// ResolveInput selects the declared output kind from the latest matching producing
// stage entry. Other inputs do not gate a consumer that only needs this binding.
func ResolveInput(definition workflowmodel.Definition, binding workflowmodel.ArtifactInput, available []artifactmodel.Revision) ([]artifactmodel.Revision, error) {
	source, ok := definition.Stage(binding.SourceStageID)
	if !ok {
		return nil, fmt.Errorf("artifact input source unavailable")
	}
	kind := ""
	for _, output := range source.ArtifactOutputs {
		if output.Name == binding.OutputName {
			kind = output.Kind
		}
	}
	if kind == "" {
		return nil, fmt.Errorf("artifact input output unavailable")
	}
	var latest int64
	for _, revision := range available {
		if revision.Kind == kind && revision.StageID == binding.SourceStageID && revision.OutputName == binding.OutputName && revision.StageVersion > latest {
			latest = revision.StageVersion
		}
	}
	revisions := []artifactmodel.Revision{}
	for _, revision := range available {
		if revision.Kind == kind && revision.StageID == binding.SourceStageID && revision.OutputName == binding.OutputName && revision.StageVersion == latest {
			revisions = append(revisions, revision)
		}
	}
	if binding.Required && len(revisions) == 0 {
		return nil, fmt.Errorf("required artifact input %q is unavailable", binding.Name)
	}
	return revisions, nil
}
