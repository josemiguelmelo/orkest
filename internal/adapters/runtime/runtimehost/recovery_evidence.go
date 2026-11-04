package runtimehost

import (
	"strings"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

// historyCollector is connection-local and receives only LoadSession replay
// observations. Live subscribers never see restoration history.
type historyCollector struct {
	sessionID        string
	coverageVerified bool
	userText         string
	userChunk        bool
	toolActivity     bool
	complete         bool
	uncertain        bool
}

func newHistoryCollector(sessionID string, coverageVerified bool) *historyCollector {
	return &historyCollector{sessionID: sessionID, coverageVerified: coverageVerified}
}

func (c *historyCollector) Add(observation agentruntime.HistoryObservation) {
	if c == nil || c.complete {
		return
	}
	switch observation.Kind {
	case agentruntime.HistoryUserChunk:
		if !c.userChunk {
			c.userText = ""
			c.toolActivity = false
		}
		c.userChunk = true
		c.userText += observation.Text
	case agentruntime.HistoryToolActivity:
		c.userChunk = false
		c.toolActivity = true
	case agentruntime.HistoryUnknown, agentruntime.HistoryIncomplete:
		c.userChunk = false
		c.uncertain = true
	case agentruntime.HistoryReplayDone:
		c.complete = true
	default:
		c.uncertain = true
	}
}

func (c *historyCollector) Decide(command interaction.Command) agentruntime.RecoveryDecision {
	if c == nil || strings.TrimSpace(c.sessionID) == "" || command.ID == "" || command.Text == "" || !c.complete || c.uncertain || !c.coverageVerified {
		return agentruntime.RecoveryDecision{Action: agentruntime.RecoveryPause, Reason: "runtime history is missing, incomplete, or unsupported"}
	}
	if c.userText != interaction.EncodeCommandPrompt(command.ID, command.Text) || c.toolActivity {
		return agentruntime.RecoveryDecision{Action: agentruntime.RecoveryPause, Reason: "runtime history does not establish a safe continuation"}
	}
	return agentruntime.RecoveryDecision{Action: agentruntime.RecoveryContinue, Reason: "matching command identity restored with no recorded tool activity"}
}
