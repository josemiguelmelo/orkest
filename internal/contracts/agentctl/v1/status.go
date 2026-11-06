package agentctlv1

import (
	"time"
)

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
