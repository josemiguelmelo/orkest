// Package workflow defines workspace stages and deterministic transition rules.
package workflow

import (
	"strings"

	"github.com/josemiguelmelo/orkest/internal/artifact"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
)

type ArtifactOutput struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type ArtifactInput struct {
	Name          string `json:"name"`
	SourceStageID string `json:"sourceStageId"`
	OutputName    string `json:"outputName"`
	Required      bool   `json:"required"`
}

type Stage struct {
	ArtifactInputs     []ArtifactInput  `json:"artifactInputs,omitempty"`
	ArtifactOutputs    []ArtifactOutput `json:"artifactOutputs,omitempty"`
	ID                 string           `json:"id"`
	Name               string           `json:"name"`
	Description        string           `json:"description"`
	ResponsibleAgentID string           `json:"responsibleAgentId"`
}

// Definition is ordered; IDs remain stable when names and positions change.
type Definition struct {
	ConfiguredBy   string           `json:"configuredBy,omitempty"`
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Version        int64            `json:"version"`
	InitialStageID string           `json:"initialStageId"`
	ClosedStageID  string           `json:"closedStageId"`
	Rules          []AutomationRule `json:"rules"`
	Stages         []Stage          `json:"stages"`
}

func (d Definition) StartStageID() string { return d.InitialStageID }
func (d Definition) FinalStageID() string { return d.ClosedStageID }

func (d Definition) Stage(id string) (Stage, bool) {
	for _, s := range d.Stages {
		if s.ID == id {
			return s, true
		}
	}
	return Stage{}, false
}

func (d *Definition) Validate() error {
	d.Name = strings.TrimSpace(d.Name)
	if d.ID == "" || d.Name == "" || len(d.Name) > 200 || len(d.Stages) == 0 || len(d.Stages) > 50 || d.Version < 1 {
		return modelerrors.ErrInvalid
	}
	seen := make(map[string]bool)
	for i := range d.Stages {
		s := &d.Stages[i]
		s.Name = strings.TrimSpace(s.Name)
		s.Description = strings.TrimSpace(s.Description)
		if s.ID == "" || len(s.ID) > 100 || strings.TrimSpace(s.ID) != s.ID || seen[s.ID] || s.Name == "" || len(s.Name) > 200 || len(s.Description) > 2000 {
			return modelerrors.ErrInvalid
		}
		seen[s.ID] = true
	}
	if !seen[d.InitialStageID] || !seen[d.ClosedStageID] {
		return modelerrors.ErrInvalid
	}
	if d.Rules == nil {
		d.Rules = []AutomationRule{}
	}
	if err := validateArtifacts(*d); err != nil {
		return err
	}
	return validateRules(*d)
}

func DefaultDefinition(name string) Definition {
	return Definition{
		Name:           name,
		Version:        1,
		InitialStageID: "todo", ClosedStageID: "done",
		Rules: []AutomationRule{
			{ID: "start-progress", Enabled: true, Event: StageEntered, SourceStageID: "in_progress", Action: StartWork},
			{ID: "review-success", Enabled: true, Event: RunSucceeded, SourceStageID: "in_progress", Action: MoveTask, TargetStageID: "in_review"},
			{ID: "review-complete", Enabled: true, Event: StageCompleted, SourceStageID: "in_progress", Action: MoveTask, TargetStageID: "in_review"},
		},
		Stages: []Stage{
			{ID: "todo", Name: "Todo", Description: "Ready to pick up"},
			{ID: "in_progress", Name: "In progress", Description: "Work in progress"},
			{ID: "in_review", Name: "In review", Description: "Ready for review"},
			{ID: "done", Name: "Done", Description: "Completed work"},
		},
	}
}

func validateArtifacts(d Definition) error {
	for _, stage := range d.Stages {
		if len(stage.ArtifactInputs) > 50 || len(stage.ArtifactOutputs) > 50 {
			return modelerrors.ErrInvalid
		}
		seen := map[string]bool{}
		for _, output := range stage.ArtifactOutputs {
			if !artifact.ValidName(output.Name) || !artifact.ValidName(output.Kind) || seen[output.Name] {
				return modelerrors.ErrInvalid
			}
			seen[output.Name] = true
		}
		seen = map[string]bool{}
		for _, input := range stage.ArtifactInputs {
			if !artifact.ValidName(input.Name) || seen[input.Name] {
				return modelerrors.ErrInvalid
			}
			seen[input.Name] = true
			source, ok := d.Stage(input.SourceStageID)
			if !ok || source.ID == stage.ID {
				return modelerrors.ErrInvalid
			}
			found := false
			for _, output := range source.ArtifactOutputs {
				if output.Name == input.OutputName {
					found = true
				}
			}
			if !found {
				return modelerrors.ErrInvalid
			}
		}
	}
	return nil
}
