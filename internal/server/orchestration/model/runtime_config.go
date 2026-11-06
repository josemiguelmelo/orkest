package model

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/josemiguelmelo/orkest/internal/contracts/configvalidation"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
)

// AgentDefinition is the portable Markdown instruction set selected for a run.
// Runtime profiles map it to the files and options their runtime supports.
type AgentDefinition struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	Instructions     string `json:"instructions"`
	PermissionPolicy string `json:"permissionPolicy,omitempty"`
}

const (
	PermissionPolicyAsk        = "ask"
	PermissionPolicyAutonomous = "autonomous"
)

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

const OrkestMCPServerID = "orkest-agent-tools"

// Skill is an immutable run snapshot of one selected SKILL.md definition.
type Skill struct {
	ID      string `json:"id"`
	SkillMD string `json:"skillMD"`
}

// GitCredential grants push access to one registered HTTPS repository for a run.
// The token is sensitive and must never be included in logs or ordinary responses.
type GitCredential struct {
	ProviderName   string          `json:"providerName"`
	ProviderConfig json.RawMessage `json:"providerConfig,omitempty"`
	RepositoryURL  string          `json:"repositoryUrl"`
	RepositoryPath string          `json:"repositoryPath"`
	Token          string          `json:"token"`
}

// ValidationError describes a rejected field without retaining its value.
type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	reason := e.Reason
	if reason == "" {
		reason = "is required"
	}
	return fmt.Sprintf("%s %s", e.Field, reason)
}

// RuntimeConfig contains only configuration consumed by a native runtime.
type RuntimeConfig struct {
	OrganizationID         string              `json:"organizationId"`
	WorkspaceID            string              `json:"workspaceId"`
	TaskID                 string              `json:"taskId"`
	TaskSessionID          string              `json:"taskSessionId,omitempty"`
	RunID                  string              `json:"runId"`
	Runtime                coreruntime.Runtime `json:"runtime"`
	Agent                  AgentDefinition     `json:"agent"`
	Model                  ModelSelection      `json:"model"`
	MCPServers             []MCPServer         `json:"mcpServers,omitempty"`
	Skills                 []Skill             `json:"skills,omitempty"`
	Environment            map[string]string   `json:"environment,omitempty"`
	GitCredentials         []GitCredential     `json:"gitCredentials,omitempty"`
	GitCredentialsRequired bool                `json:"gitCredentialsRequired,omitempty"`
}

func (c RuntimeConfig) Clone() RuntimeConfig {
	c.MCPServers = slices.Clone(c.MCPServers)
	for i := range c.MCPServers {
		c.MCPServers[i].Headers = maps.Clone(c.MCPServers[i].Headers)
	}
	c.Skills = slices.Clone(c.Skills)
	c.Environment = maps.Clone(c.Environment)
	c.GitCredentials = slices.Clone(c.GitCredentials)
	for i := range c.GitCredentials {
		c.GitCredentials[i].ProviderConfig = slices.Clone(c.GitCredentials[i].ProviderConfig)
	}
	return c
}

func (c RuntimeConfig) Validate() error {
	if issue := configvalidation.Required(
		[2]string{"organizationId", c.OrganizationID},
		[2]string{"workspaceId", c.WorkspaceID},
		[2]string{"taskId", c.TaskID},
		[2]string{"runId", c.RunID},
		[2]string{"runtime", string(c.Runtime)},
		[2]string{"model.provider", c.Model.Provider},
		[2]string{"model.id", c.Model.ID},
		[2]string{"agent.name", c.Agent.Name},
		[2]string{"agent.instructions", c.Agent.Instructions},
	); issue != nil {
		return validationError(issue)
	}
	if issue := configvalidation.Runtime(c.Runtime, c.Model.Provider, c.Agent.PermissionPolicy); issue != nil {
		return validationError(issue)
	}
	for _, server := range c.MCPServers {
		if issue := configvalidation.MCPServer(server.ID, server.Name, server.URL, server.Headers); issue != nil {
			return validationError(issue)
		}
	}
	seen := make(map[string]bool, len(c.Skills))
	for _, skill := range c.Skills {
		if issue := configvalidation.Skill(skill.ID, skill.SkillMD, seen[skill.ID]); issue != nil {
			return validationError(issue)
		}
		seen[skill.ID] = true
	}
	for _, credential := range c.GitCredentials {
		if issue := configvalidation.GitCredential(credential.ProviderName, credential.Token, credential.RepositoryPath, credential.RepositoryURL, credential.ProviderConfig); issue != nil {
			return validationError(issue)
		}
	}
	return validationError(configvalidation.Environment(c.Environment))
}

func validationError(issue *configvalidation.Issue) error {
	if issue == nil {
		return nil
	}
	return &ValidationError{Field: issue.Field, Reason: issue.Reason}
}

func ValidSkillID(value string) bool { return configvalidation.ValidSkillID(value) }
