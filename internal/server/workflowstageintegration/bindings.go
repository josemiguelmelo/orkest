package workflowstageintegration

import (
	"context"
	"encoding/json"

	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
)

func (s *WorkflowStageIntegrationService) ValidateBindings(ctx context.Context, workspaceID string, d workflowmodel.Definition) error {
	for _, rule := range d.Rules {
		if rule.Integration == nil {
			continue
		}
		c, err := s.repo.GetConnection(ctx, workspaceID, rule.Integration.ConnectionID)
		if err != nil {
			return err
		}
		p := s.providers[c.Connection.Provider]
		if p == nil {
			return problem.ErrInvalid
		}
		if err := p.ValidateParameters(rule.Integration.ActionID, rule.Integration.Parameters); err != nil {
			return err
		}
		var common struct {
			ArtifactInputName string `json:"artifactInputName"`
		}
		if err := json.Unmarshal(rule.Integration.Parameters, &common); err != nil {
			return problem.ErrInvalid
		}
		stage, ok := d.Stage(rule.SourceStageID)
		if !ok {
			return problem.ErrInvalid
		}
		found := false
		for _, input := range stage.ArtifactInputs {
			if input.Name == common.ArtifactInputName {
				source, sourceExists := d.Stage(input.SourceStageID)
				if sourceExists {
					for _, output := range source.ArtifactOutputs {
						if output.Name == input.OutputName && (output.Kind == "code_review" || output.Kind == "pull_request") {
							found = true
						}
					}
				}
			}
		}
		if !found {
			return problem.ErrInvalid
		}
	}
	return nil
}
