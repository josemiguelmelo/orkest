package tasktriggerintegration

import "context"

// Ingest stores verified events with the configuration version used to decode
// them. Task association and workflow decisions belong to their own services.
func (s *TaskTriggerIntegrationService) Ingest(ctx context.Context, integration Integration, delivery string, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	status := "awaiting_link"
	if !integration.Enabled {
		status = "ignored"
	}
	return s.repository.EnqueueEvents(ctx, integration.ID, integration.Version, delivery, events, status)
}
