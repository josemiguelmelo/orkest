package pullrequestlink

import (
	"reflect"
	"testing"
	"time"

	trigger "github.com/josemiguelmelo/orkest/internal/server/tasktriggerintegration"
)

func TestResolutionPolicy(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name               string
		change             func(*ResolutionSnapshot)
		status, diagnostic string
		associate          bool
	}{
		{"associate", func(*ResolutionSnapshot) {}, "pending", "", true},
		{"disabled", func(s *ResolutionSnapshot) { s.Integration.Enabled = false }, "ignored", "integration_disabled", false},
		{"suppressed", func(s *ResolutionSnapshot) { s.Suppressed = true }, "ignored", "association_suppressed", false},
		{"missing evidence", func(s *ResolutionSnapshot) { s.Candidates = nil }, "awaiting_link", "waiting_for_branch_evidence", false},
		{"expired", func(s *ResolutionSnapshot) { s.Candidates = nil; s.Delivery.ReceivedAt = now.Add(-24 * time.Hour) }, "ignored", "association_expired", false},
		{"branch conflict", func(s *ResolutionSnapshot) {
			other := s.Candidates[0]
			other.TaskID = "other"
			s.Candidates = append(s.Candidates, other)
		}, "blocked", "conflicting_branch_ownership", false},
		{"existing ownership", func(s *ResolutionSnapshot) { s.Candidates = nil; s.ExistingTaskID = "task" }, "pending", "", true},
		{"unselected", func(s *ResolutionSnapshot) { s.Tasks[0].RepositorySelected = false }, "blocked", "repository_not_selected_or_link_conflict", false},
		{"old stage", func(s *ResolutionSnapshot) { s.Tasks[0].StageEnteredAt = now.Add(2 * time.Second) }, "ignored", "stale_stage_entry", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := ResolutionSnapshot{Integration: trigger.Integration{Enabled: true}, Delivery: trigger.Delivery{ReceivedAt: now, Event: trigger.Event{RepositoryID: "repo", HeadRepositoryID: "repo", HeadBranch: "skill/name", HeadCommitID: "sha", OccurredAt: now}}, Candidates: []Candidate{{EligibleForLinking: true, TaskID: "task", RepositoryID: "repo", HeadRepositoryID: "repo", Branch: "skill/name", CommitID: "sha"}}, Tasks: []TaskState{{TaskID: "task", RepositorySelected: true, StageVersion: 3}}}
			tc.change(&snapshot)
			before := PlanResolution(snapshot, now)
			result := PlanResolution(snapshot, now)
			if !reflect.DeepEqual(before, result) {
				t.Fatal("planner changed input or depends on hidden state")
			}
			if result.Status != tc.status || result.Diagnostic != tc.diagnostic || result.Associate != tc.associate {
				t.Fatalf("decision: %+v", result)
			}
			if result.Status == "awaiting_link" && result.RetryAt != now.Add(5*time.Second) {
				t.Fatalf("retry: %+v", result)
			}
		})
	}
}
