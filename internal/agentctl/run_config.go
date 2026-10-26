package agentctl

import (
	"fmt"
	"strings"

	coreruntime "github.com/josemiguelmelo/orkest/internal/core/runtime"
)

// RunConfig is the resolved configuration bound to one task execution run.
// It is separate from reusable agent definitions and process startup options.
type RunConfig struct {
	OrganizationID string              `json:"organizationId"`
	WorkspaceID    string              `json:"workspaceId"`
	TaskID         string              `json:"taskId"`
	RunID          string              `json:"runId"`
	Runtime        coreruntime.Runtime `json:"runtime"`
	Agent          AgentDefinition     `json:"agent"`
	Model          ModelSelection      `json:"model"`
}

// AgentDefinition is the portable Markdown instruction set selected for a run.
// Runtime adapters map it to the files and options their runtime supports.
type AgentDefinition struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
}

type ModelSelection struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
}

// ValidationError identifies a missing field without retaining its value.
type ValidationError struct{ Field string }

func (e *ValidationError) Error() string { return fmt.Sprintf("%s is required", e.Field) }

// Validate checks the resolved run configuration without starting execution.
func (c RunConfig) Validate() error {
	for _, field := range []struct{ name, value string }{
		{"organizationId", c.OrganizationID},
		{"workspaceId", c.WorkspaceID},
		{"taskId", c.TaskID},
		{"runId", c.RunID},
		{"runtime", string(c.Runtime)},
		{"model.provider", c.Model.Provider},
		{"model.id", c.Model.ID},
		{"agent.name", c.Agent.Name},
		{"agent.instructions", c.Agent.Instructions},
	} {
		if strings.TrimSpace(field.value) == "" {
			return &ValidationError{Field: field.name}
		}
	}
	if !c.Runtime.Available() {
		return &ValidationError{Field: "runtime"}
	}

	return nil
}
