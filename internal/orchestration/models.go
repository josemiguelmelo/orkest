// Package orchestration coordinates task run startup and agentctl communication.
package orchestration

import (
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

type LaunchStatus string

const (
	LaunchStarting              LaunchStatus = "starting"
	LaunchProcessStarted        LaunchStatus = "process_started"
	LaunchReady                 LaunchStatus = "ready"
	LaunchFailed                LaunchStatus = "failed"
	LaunchCompletedPendingClose LaunchStatus = "completed_pending_close"
	LaunchFailedPendingClose    LaunchStatus = "failed_pending_close"
	LaunchCompleted             LaunchStatus = "completed"
)

type Process struct {
	ID       string
	Endpoint string
}

type Launch struct {
	RunID          string
	OrganizationID string
	WorkspaceID    string
	TaskID         string
	Status         LaunchStatus
	ProcessID      string
	Endpoint       string
	FailureStage   string
	FailureReason  string
	LastSequence   uint64
	LastEventType  string
	UpdatedAt      time.Time
}

type Event struct {
	RunID    string         `json:"runId"`
	Sequence uint64         `json:"sequence"`
	Payload  agentctl.Event `json:"payload"`
}
