// Package orchestration coordinates task run startup and agentctl communication.
package orchestration

import (
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

type LaunchStatus string

const (
	LaunchStarting       LaunchStatus = "starting"
	LaunchProcessStarted LaunchStatus = "process_started"
	LaunchReady          LaunchStatus = "ready"
	LaunchFailed         LaunchStatus = "failed"
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
	UpdatedAt      time.Time
}

type Event struct {
	RunID    string
	Sequence uint64
	Payload  agentctl.Event
}
