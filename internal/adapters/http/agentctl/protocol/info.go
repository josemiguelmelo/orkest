// Package protocol defines the shared agentctl HTTP wire representations.
package protocol

import (
	"runtime"
	"time"

	runmodel "github.com/josemiguelmelo/orkest/internal/agentctl/run/model"
	wire "github.com/josemiguelmelo/orkest/internal/contracts/agentctl/v1"
)

func Snapshot(status runmodel.RunStatus, startedAt time.Time) wire.Info {
	return wire.Info{
		OrganizationID:     status.OrganizationID,
		WorkspaceID:        status.WorkspaceID,
		TaskID:             status.TaskID,
		RunID:              status.RunID,
		RecoveryBlocked:    status.RecoveryBlocked,
		State:              string(status.State),
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
