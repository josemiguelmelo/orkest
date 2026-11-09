package tasktriggerintegration

import (
	"context"
	"time"

	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
)

type IngestionSnapshot struct {
	Integration    Integration
	Event          Event
	LinkedTaskID   string
	LinkCreatedAt  time.Time
	StageVersion   int64
	StageEnteredAt time.Time
}

type IngestionDecision struct {
	Status       string
	TaskID       string
	StageVersion int64
}

func PlanIngestion(s IngestionSnapshot) IngestionDecision {
	if !s.Integration.Enabled {
		return IngestionDecision{Status: "ignored"}
	}
	if s.LinkedTaskID == "" || s.LinkCreatedAt.After(s.Event.OccurredAt) || s.Event.OccurredAt.Add(time.Second).Before(s.StageEnteredAt) {
		return IngestionDecision{Status: "awaiting_link"}
	}
	return IngestionDecision{Status: "pending", TaskID: s.LinkedTaskID, StageVersion: s.StageVersion}
}

// Ingest accepts provider-verified events from webhook decoding or authenticated
// reconciliation. Each event's policy is planned before the conditional write.
func (s *TaskTriggerIntegrationService) Ingest(ctx context.Context, i Integration, delivery string, events []Event) error {
	for _, event := range events {
		snapshot, err := s.repository.IngestionSnapshot(ctx, i.ID, event)
		if err != nil {
			return err
		}
		if snapshot.Integration.Version != i.Version {
			return problem.ErrConflict
		}
		decision := PlanIngestion(snapshot)
		if err = s.repository.CommitIngestion(ctx, snapshot, delivery, decision); err != nil {
			return err
		}
	}
	return nil
}
