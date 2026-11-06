package agentctlv1

import (
	"time"
)

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

type StreamFrame struct {
	Type  string `json:"type"`
	Info  *Info  `json:"info,omitempty"`
	Event *Event `json:"event,omitempty"`
}
