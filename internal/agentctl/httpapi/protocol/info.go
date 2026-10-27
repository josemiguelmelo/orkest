// Package protocol defines the shared agentctl HTTP wire representations.
package protocol

import (
	"runtime"
	"time"

	agentrun "github.com/josemiguelmelo/orkest/internal/agentctl/run"
)

type Info struct {
	OrganizationID     string                  `json:"organizationId,omitempty"`
	WorkspaceID        string                  `json:"workspaceId,omitempty"`
	TaskID             string                  `json:"taskId,omitempty"`
	RunID              string                  `json:"runId,omitempty"`
	RecoveryBlocked    bool                    `json:"recoveryBlocked"`
	State              agentrun.ExecutionState `json:"state,omitempty"`
	ProcessAlive       bool                    `json:"processAlive"`
	SessionID          string                  `json:"sessionId,omitempty"`
	TaskSessionID      string                  `json:"taskSessionId,omitempty"`
	ActiveCommandID    string                  `json:"activeCommand,omitempty"`
	QueueDepth         int                     `json:"queueDepth"`
	QueuePaused        bool                    `json:"queuePaused"`
	RunEnded           bool                    `json:"runEnded"`
	Service            string                  `json:"service"`
	APIVersion         string                  `json:"apiVersion"`
	GoVersion          string                  `json:"goVersion"`
	OS                 string                  `json:"os"`
	Arch               string                  `json:"arch"`
	StartedAt          time.Time               `json:"startedAt"`
	Configured         bool                    `json:"configured"`
	ExecutionAvailable bool                    `json:"executionAvailable"`
}

func Snapshot(status agentrun.RunStatus, startedAt time.Time) Info {
	return Info{
		OrganizationID:     status.OrganizationID,
		WorkspaceID:        status.WorkspaceID,
		TaskID:             status.TaskID,
		RunID:              status.RunID,
		RecoveryBlocked:    status.RecoveryBlocked,
		State:              status.State,
		ProcessAlive:       status.ProcessAlive,
		SessionID:          status.SessionID,
		TaskSessionID:      status.TaskSessionID,
		ActiveCommandID:    status.ActiveCommandID,
		QueueDepth:         status.QueueDepth,
		QueuePaused:        status.QueuePaused,
		RunEnded:           status.RunEnded,
		Service:            "agentctl",
		APIVersion:         "v1",
		GoVersion:          runtime.Version(),
		OS:                 runtime.GOOS,
		Arch:               runtime.GOARCH,
		StartedAt:          startedAt,
		Configured:         status.Configured,
		ExecutionAvailable: status.ExecutionAvailable,
	}
}
