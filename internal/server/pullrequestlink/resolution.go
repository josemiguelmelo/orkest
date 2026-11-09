package pullrequestlink

import (
	"time"

	trigger "github.com/josemiguelmelo/orkest/internal/server/tasktriggerintegration"
)

const (
	resolutionLifetime   = 24 * time.Hour
	resolutionRetryDelay = 5 * time.Second
)

type TaskState struct {
	TaskID             string
	RepositorySelected bool
	StageVersion       int64
	StageEnteredAt     time.Time
}

type ResolutionSnapshot struct {
	AvailableAt    time.Time
	Integration    trigger.Integration
	Delivery       trigger.Delivery
	Suppressed     bool
	ExistingTaskID string
	Candidates     []Candidate
	Tasks          []TaskState
}

type Resolution struct {
	Status       string
	Diagnostic   string
	TaskID       string
	StageVersion int64
	Associate    bool
	RetryAt      time.Time
}

// PlanResolution chooses outcomes using only supplied facts and time. It neither
// reads persistence nor mutates its input; the service explicitly commits the result.
func PlanResolution(s ResolutionSnapshot, now time.Time) Resolution {
	result := Resolution{Status: "ignored"}
	if !s.Integration.Enabled {
		result.Diagnostic = "integration_disabled"
		return result
	}
	if s.Suppressed {
		result.Diagnostic = "association_suppressed"
		return result
	}
	event := s.Delivery.Event
	taskID := s.ExistingTaskID
	if taskID == "" {
		var diagnostic string
		taskID, diagnostic = Match(PullRequest{RepositoryID: event.RepositoryID, HeadRepositoryID: event.HeadRepositoryID, Branch: event.HeadBranch, CommitID: event.HeadCommitID}, s.Candidates)
		if taskID == "" {
			if diagnostic == "conflicting_branch_ownership" {
				return Resolution{Status: "blocked", Diagnostic: diagnostic}
			}
			if !now.Before(s.Delivery.ReceivedAt.Add(resolutionLifetime)) {
				return Resolution{Status: "ignored", Diagnostic: "association_expired"}
			}
			return Resolution{Status: "awaiting_link", Diagnostic: diagnostic, RetryAt: now.Add(resolutionRetryDelay)}
		}
	}
	var state TaskState
	for _, candidate := range s.Tasks {
		if candidate.TaskID == taskID {
			state = candidate
			break
		}
	}
	if !state.RepositorySelected {
		return Resolution{Status: "blocked", Diagnostic: "repository_not_selected_or_link_conflict"}
	}
	result.TaskID, result.StageVersion, result.Associate = taskID, state.StageVersion, true
	if event.OccurredAt.Add(time.Second).Before(state.StageEnteredAt) {
		result.Diagnostic = "stale_stage_entry"
		return result
	}
	result.Status = "pending"
	return result
}
