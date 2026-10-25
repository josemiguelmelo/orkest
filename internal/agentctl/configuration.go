package agentctl

import (
	"errors"
	"strings"
)

type Configuration struct {
	OrganizationID string `json:"organizationId"`
	WorkspaceID    string `json:"workspaceId"`
	TaskID         string `json:"taskId"`
	RunID          string `json:"runId"`
	Runtime        string `json:"runtime"`
	Model          Model  `json:"model"`
	SystemPrompt   string `json:"systemPrompt"`
}

type Model struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
}

// Validate checks the resolved run configuration without starting execution.
func (c Configuration) Validate() error {
	for _, value := range []string{c.OrganizationID, c.WorkspaceID, c.TaskID, c.RunID, c.Model.Provider, c.Model.ID, c.SystemPrompt} {
		if strings.TrimSpace(value) == "" {
			return errors.New("scope, model and systemPrompt are required")
		}
	}
	if c.Runtime != "opencode" {
		return errors.New("only opencode runtime is supported")
	}
	return nil
}
