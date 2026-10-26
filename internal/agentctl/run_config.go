package agentctl

import (
	"fmt"
	"net/url"
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
	MCPServers     []MCPServer         `json:"mcpServers,omitempty"`
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

// MCPServer is an HTTP MCP endpoint resolved for this run. Headers are sensitive.
type MCPServer struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
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
	for _, server := range c.MCPServers {
		parsed, err := url.Parse(server.URL)
		if strings.TrimSpace(server.ID) == "" || strings.TrimSpace(server.Name) == "" || err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			return &ValidationError{Field: "mcpServers"}
		}
		for name, value := range server.Headers {
			if strings.TrimSpace(name) == "" || strings.ContainsAny(name+value, "\r\n") {
				return &ValidationError{Field: "mcpServers.headers"}
			}
		}
	}

	return nil
}
