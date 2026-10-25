// Package protocol defines the shared agentctl HTTP wire representations.
package protocol

import (
	"runtime"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

type Info struct {
	State              agentctl.State `json:"state,omitempty"`
	ProcessAlive       bool           `json:"processAlive"`
	SessionID          string         `json:"sessionId,omitempty"`
	Service            string         `json:"service"`
	APIVersion         string         `json:"apiVersion"`
	GoVersion          string         `json:"goVersion"`
	OS                 string         `json:"os"`
	Arch               string         `json:"arch"`
	StartedAt          time.Time      `json:"startedAt"`
	Configured         bool           `json:"configured"`
	ExecutionAvailable bool           `json:"executionAvailable"`
}

func Snapshot(status agentctl.Status, startedAt time.Time) Info {
	return Info{
		State:              status.State,
		ProcessAlive:       status.ProcessAlive,
		SessionID:          status.SessionID,
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
