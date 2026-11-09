package tasktriggerintegration

import (
	"testing"
	"time"
)

func TestIngestionPolicy(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		change func(*IngestionSnapshot)
		status string
	}{
		{"linked", func(*IngestionSnapshot) {}, "pending"},
		{"disabled", func(s *IngestionSnapshot) { s.Integration.Enabled = false }, "ignored"},
		{"unlinked", func(s *IngestionSnapshot) { s.LinkedTaskID = "" }, "awaiting_link"},
		{"link too late", func(s *IngestionSnapshot) { s.LinkCreatedAt = now.Add(time.Second) }, "awaiting_link"},
		{"stale stage", func(s *IngestionSnapshot) { s.StageEnteredAt = now.Add(2 * time.Second) }, "awaiting_link"},
		{"timestamp tolerance", func(s *IngestionSnapshot) { s.StageEnteredAt = now.Add(time.Second) }, "pending"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := IngestionSnapshot{Integration: Integration{Enabled: true}, Event: Event{OccurredAt: now}, LinkedTaskID: "task", LinkCreatedAt: now.Add(-time.Minute), StageVersion: 2}
			tc.change(&s)
			result := PlanIngestion(s)
			if result.Status != tc.status {
				t.Fatalf("%+v", result)
			}
			if result.Status != "pending" && (result.TaskID != "" || result.StageVersion != 0) {
				t.Fatal("ineligible event acquired task binding")
			}
		})
	}
}

func TestResourceLinkPolicy(t *testing.T) {
	snapshot := ResourceLinkSnapshot{Link: ResourceLink{TaskID: "task"}, RepositorySelected: true}
	change, err := PlanResourceLink(snapshot, false)
	if err != nil || !change.Linked || change.Suppressed {
		t.Fatalf("link: %+v %v", change, err)
	}
	snapshot.ExistingTaskID = "other"
	if _, err := PlanResourceLink(snapshot, false); err == nil {
		t.Fatal("ownership transferred")
	}
	change, err = PlanResourceLink(snapshot, true)
	if err != nil || change.Linked || !change.Suppressed {
		t.Fatalf("unlink: %+v %v", change, err)
	}
	snapshot.ExistingTaskID = ""
	snapshot.RepositorySelected = false
	if _, err := PlanResourceLink(snapshot, false); err == nil {
		t.Fatal("unselected repository linked")
	}
}
