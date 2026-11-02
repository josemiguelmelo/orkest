package workflow

import modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"

// ConfigurationSnapshot contains reference facts loaded under the configuration lock.
type ConfigurationSnapshot struct {
	Current          *Definition
	AgentIDs         []string
	OccupiedStageIDs []string
}

func PlanConfigurationChange(snapshot ConfigurationSnapshot, input Definition) (Definition, error) {
	if snapshot.Current != nil {
		if input.Version != snapshot.Current.Version {
			return Definition{}, modelerrors.ErrConflict
		}
		if input.Name == "" {
			input.Name = snapshot.Current.Name
		}
	}
	if err := input.Validate(); err != nil {
		return Definition{}, err
	}
	agents := make(map[string]bool, len(snapshot.AgentIDs))
	for _, id := range snapshot.AgentIDs {
		agents[id] = true
	}
	for _, stage := range input.Stages {
		if stage.ResponsibleAgentID != "" && !agents[stage.ResponsibleAgentID] {
			return Definition{}, modelerrors.ErrInvalid
		}
	}
	for _, id := range snapshot.OccupiedStageIDs {
		if _, exists := input.Stage(id); !exists {
			return Definition{}, modelerrors.ErrConflict
		}
	}
	return input, nil
}

type WorkflowDeletionSnapshot struct {
	WorkflowIDs []string
	Referenced  bool
}

func ValidateWorkflowDeletion(snapshot WorkflowDeletionSnapshot, id string) error {
	exists := false
	for _, current := range snapshot.WorkflowIDs {
		if current == id {
			exists = true
			break
		}
	}
	if !exists {
		return modelerrors.ErrNotFound
	}
	if len(snapshot.WorkflowIDs) <= 1 || snapshot.Referenced {
		return modelerrors.ErrConflict
	}
	return nil
}
