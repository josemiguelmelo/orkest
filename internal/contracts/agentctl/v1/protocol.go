// Package agentctlv1 contains stable agentctl transport values, without execution policy.
package agentctlv1

import (
	"encoding/json"
	"time"
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
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
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

type Update struct {
	Kind       string         `json:"kind"`
	Text       string         `json:"text,omitempty"`
	ToolCallID string         `json:"toolCallId,omitempty"`
	Title      string         `json:"title,omitempty"`
	Status     string         `json:"status,omitempty"`
	ToolKind   string         `json:"toolKind,omitempty"`
	Input      string         `json:"input,omitempty"`
	Output     string         `json:"output,omitempty"`
	Truncated  bool           `json:"truncated,omitempty"`
	Locations  []ToolLocation `json:"locations,omitzero"`
	Content    []ToolContent  `json:"content,omitzero"`
}

type ToolLocation struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
}

type ToolContent struct {
	Kind      string `json:"kind"`
	Path      string `json:"path,omitempty"`
	OldText   string `json:"oldText,omitempty"`
	NewText   string `json:"newText,omitempty"`
	Text      string `json:"text,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

type Event struct {
	At             time.Time   `json:"at"`
	Sequence       uint64      `json:"sequence"`
	Type           string      `json:"type"`
	OrganizationID string      `json:"organizationId"`
	WorkspaceID    string      `json:"workspaceId"`
	TaskID         string      `json:"taskId"`
	RunID          string      `json:"runId"`
	TaskSessionID  string      `json:"taskSessionId,omitempty"`
	SessionID      string      `json:"sessionId"`
	CommandID      string      `json:"commandId,omitempty"`
	RequestID      string      `json:"requestId,omitempty"`
	Text           string      `json:"text,omitempty"`
	Update         *Update     `json:"update,omitempty"`
	Permission     *Permission `json:"permission,omitempty"`
	StopReason     string      `json:"stopReason,omitempty"`
	Error          string      `json:"error,omitempty"`
}

type PermissionOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type Permission struct {
	ToolCallID string             `json:"toolCallId"`
	Title      string             `json:"title"`
	Options    []PermissionOption `json:"options"`
}

type PermissionResponse struct {
	TaskID    string `json:"taskId"`
	RunID     string `json:"runId"`
	SessionID string `json:"sessionId"`
	RequestID string `json:"requestId"`
	OptionID  string `json:"optionId"`
}

type RunConfig struct {
	RuntimeConfig
	ExecutionProfileID   string `json:"executionProfileId,omitempty"`
	ExecutionImage       string `json:"executionImage,omitempty"`
	RestoreSession       bool   `json:"restoreSession,omitempty"`
	EncryptedEnvironment string `json:"encryptedEnvironment,omitempty"`
}

type Info struct {
	OrganizationID     string    `json:"organizationId,omitempty"`
	WorkspaceID        string    `json:"workspaceId,omitempty"`
	TaskID             string    `json:"taskId,omitempty"`
	RunID              string    `json:"runId,omitempty"`
	RecoveryBlocked    bool      `json:"recoveryBlocked"`
	State              string    `json:"state,omitempty"`
	ProcessAlive       bool      `json:"processAlive"`
	SessionID          string    `json:"sessionId,omitempty"`
	TaskSessionID      string    `json:"taskSessionId,omitempty"`
	ActiveCommandID    string    `json:"activeCommand,omitempty"`
	QueueDepth         int       `json:"queueDepth"`
	QueuePaused        bool      `json:"queuePaused"`
	RunEnded           bool      `json:"runEnded"`
	Service            string    `json:"service"`
	APIVersion         string    `json:"apiVersion"`
	GoVersion          string    `json:"goVersion"`
	OS                 string    `json:"os"`
	Arch               string    `json:"arch"`
	StartedAt          time.Time `json:"startedAt"`
	Configured         bool      `json:"configured"`
	ExecutionAvailable bool      `json:"executionAvailable"`
}

type PromptRequest struct {
	CommandID string `json:"commandId"`
	Text      string `json:"text"`
}
type (
	CancelRequest struct {
		CommandID string `json:"commandId"`
	}
	CommandResponse struct {
		Status    string `json:"status"`
		CommandID string `json:"commandId,omitempty"`
		RequestID string `json:"requestId,omitempty"`
	}
)

type CommandStatus struct {
	CommandID string `json:"commandId"`
	Status    string `json:"status"`
}
type StreamFrame struct {
	Type  string `json:"type"`
	Info  *Info  `json:"info,omitempty"`
	Event *Event `json:"event,omitempty"`
}
type Error struct {
	Code    string `json:"errorCode"`
	Message string `json:"error"`
	Field   string `json:"field,omitempty"`
}
