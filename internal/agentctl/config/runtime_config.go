// Package config contains resolved run settings and their validation.
package config

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"

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

var (
	skillIDPattern         = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	environmentNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

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
		return &ValidationError{Field: "runtime", Reason: "is unsupported"}
	}
	if c.Agent.PermissionPolicy != "" && c.Agent.PermissionPolicy != PermissionPolicyAsk && c.Agent.PermissionPolicy != PermissionPolicyAutonomous {
		return &ValidationError{Field: "agent.permissionPolicy", Reason: "is unsupported"}
	}
	if provider, ok := coreruntime.ProviderFor(c.Runtime); ok {
		if err := provider.ValidateDefinition(coreruntime.Definition{ModelProvider: c.Model.Provider}); err != nil {
			return &ValidationError{Field: "model.provider", Reason: "is invalid for runtime"}
		}
	}
	if err := validateMCPServers(c.MCPServers); err != nil {
		return err
	}
	if err := validateSkills(c.Skills); err != nil {
		return err
	}
	for _, credential := range c.GitCredentials {
		parsed, err := url.Parse(credential.RepositoryURL)
		if strings.TrimSpace(credential.ProviderName) == "" || strings.TrimSpace(credential.Token) == "" || strings.TrimSpace(credential.RepositoryPath) == "" || err != nil || parsed == nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || strings.ContainsAny(credential.Token, "\x00\r\n") || (len(credential.ProviderConfig) > 0 && !json.Valid(credential.ProviderConfig)) {
			return &ValidationError{Field: "gitCredentials", Reason: "must contain valid HTTPS repository credentials"}
		}
	}
	for name, value := range c.Environment {
		if !environmentNamePattern.MatchString(name) || strings.ContainsRune(value, '\x00') {
			return &ValidationError{Field: "environment", Reason: "contains an invalid name or value"}
		}
		if strings.HasPrefix(name, "GIT_CONFIG_") {
			return &ValidationError{Field: "environment", Reason: "contains a reserved Git configuration variable"}
		}
		if strings.HasPrefix(name, "ORKEST_GH_") {
			return &ValidationError{Field: "environment", Reason: "contains a reserved GitHub CLI variable"}
		}
		if strings.HasPrefix(name, "ORKEST_GITLAB_") {
			return &ValidationError{Field: "environment", Reason: "contains a reserved GitLab CLI variable"}
		}
	}
	return nil
}

func validateMCPServers(servers []MCPServer) error {
	for _, server := range servers {
		if strings.TrimSpace(server.ID) == "" || strings.TrimSpace(server.Name) == "" {
			return &ValidationError{Field: "mcpServers", Reason: "must include an ID and name"}
		}
		parsed, err := url.Parse(server.URL)
		if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			return &ValidationError{Field: "mcpServers", Reason: "must use HTTP or HTTPS URLs without user information"}
		}
		for name, value := range server.Headers {
			if strings.TrimSpace(name) == "" || strings.ContainsAny(name+value, "\r\n") {
				return &ValidationError{Field: "mcpServers.headers", Reason: "must have nonempty names and no line breaks"}
			}
		}
	}
	return nil
}

func validateSkills(skills []Skill) error {
	seen := make(map[string]bool, len(skills))
	for _, skill := range skills {
		if !ValidSkillID(skill.ID) {
			return &ValidationError{Field: "skills", Reason: "must use lowercase alphanumeric IDs separated by single hyphens"}
		}
		if strings.TrimSpace(skill.SkillMD) == "" {
			return &ValidationError{Field: "skills", Reason: "must include a nonempty definition"}
		}
		if seen[skill.ID] {
			return &ValidationError{Field: "skills", Reason: "must have unique IDs"}
		}
		seen[skill.ID] = true
	}
	return nil
}

func ValidSkillID(value string) bool { return skillIDPattern.MatchString(value) }
