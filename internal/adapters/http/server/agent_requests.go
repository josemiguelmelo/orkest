package httpapi

import (
	agentmodel "github.com/josemiguelmelo/orkest/internal/server/agent/model"
)

type agentDefinitionRequest struct {
	agentmodel.AgentDefinition
	Configuration map[string]string `json:"configuration"`
}

type agentUpdateRequest struct {
	agentmodel.AgentDefinition
	Configuration      map[string]string `json:"configuration"`
	ExecutionProfileID *string           `json:"executionProfileId"`
	RuntimeProfileID   *string           `json:"runtimeProfileId"`
}
