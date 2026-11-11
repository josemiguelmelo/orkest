package agentctlv1

import (
	"encoding/json"
)

type AgentDefinition struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	Instructions     string `json:"instructions"`
	PermissionPolicy string `json:"permissionPolicy,omitempty"`
}

type ModelSelection struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
}

type MCPServer struct {
	Transport   string            `json:"transport"`
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers,omitempty"`
}

type Skill struct {
	ID      string `json:"id"`
	SkillMD string `json:"skillMD"`
}

type GitCredential struct {
	ProviderName   string          `json:"providerName"`
	ProviderConfig json.RawMessage `json:"providerConfig,omitempty"`
	RepositoryURL  string          `json:"repositoryUrl"`
	RepositoryPath string          `json:"repositoryPath"`
	Token          string          `json:"token"`
}

type RuntimeConfig struct {
	OrganizationID         string            `json:"organizationId"`
	WorkspaceID            string            `json:"workspaceId"`
	TaskID                 string            `json:"taskId"`
	TaskSessionID          string            `json:"taskSessionId,omitempty"`
	RunID                  string            `json:"runId"`
	Runtime                string            `json:"runtime"`
	Agent                  AgentDefinition   `json:"agent"`
	Model                  ModelSelection    `json:"model"`
	MCPServers             []MCPServer       `json:"mcpServers,omitempty"`
	Skills                 []Skill           `json:"skills,omitempty"`
	Environment            map[string]string `json:"environment,omitempty"`
	GitCredentials         []GitCredential   `json:"gitCredentials,omitempty"`
	GitCredentialsRequired bool              `json:"gitCredentialsRequired,omitempty"`
}

type RunConfig struct {
	RuntimeConfig
	ExecutionProfileID   string `json:"executionProfileId,omitempty"`
	ExecutionImage       string `json:"executionImage,omitempty"`
	RestoreSession       bool   `json:"restoreSession,omitempty"`
	EncryptedEnvironment string `json:"encryptedEnvironment,omitempty"`
}
