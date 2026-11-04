package agent

import coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"

// Runtime aliases the shared platform and agentctl runtime type.
type Runtime = coreruntime.Runtime

const (
	RuntimeOpenCode      = coreruntime.OpenCode
	RuntimeClaudeCode    = coreruntime.ClaudeCode
	RuntimeCodex         = coreruntime.Codex
	RuntimeGitHubCopilot = coreruntime.GitHubCopilot
	RuntimeKiro          = coreruntime.Kiro
)

type PermissionPolicy string

const (
	PermissionPolicyAsk        PermissionPolicy = "ask"
	PermissionPolicyAutonomous PermissionPolicy = "autonomous"
)

type Agent struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspaceId"`
	AgentDefinition
}

type AgentDefinition struct {
	ExecutionProfileID string            `json:"executionProfileId"`
	RuntimeProfileID   string            `json:"runtimeProfileId"`
	Name               string            `json:"name"`
	Runtime            Runtime           `json:"runtime"`
	Provider           string            `json:"provider"`
	Model              string            `json:"model"`
	Description        string            `json:"description"`
	SystemPrompt       string            `json:"systemPrompt"`
	MCPServerIDs       []string          `json:"mcpServerIds"`
	SkillIDs           []string          `json:"skillIds"`
	PermissionPolicy   PermissionPolicy  `json:"permissionPolicy"`
	Configuration      map[string]string `json:"-"`
	HasConfiguration   bool              `json:"hasConfiguration"`
}
