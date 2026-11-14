package postgres

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	problem "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
	stage "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration"
	stageintegrationmodel "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration/model"
)

func insertIntegrationEvents(ctx context.Context, q *sqlc.Queries, c stageintegrationmodel.Connection, events []stageintegrationmodel.ProviderEvent) error {
	for _, event := range events {
		subject, err := json.Marshal(event.Subject)
		if err != nil {
			return err
		}
		facts := event.Facts
		if len(facts) == 0 {
			facts = json.RawMessage(`{}`)
		}
		if err = q.InsertWorkflowStageProviderEvent(ctx, sqlc.InsertWorkflowStageProviderEventParams{WorkspaceID: c.WorkspaceID, ConnectionID: c.ID, SemanticID: event.ID, ActionID: event.ActionID, Subject: subject, OccurredAt: pgtype.Timestamptz{Time: event.OccurredAt, Valid: true}, Facts: facts}); err != nil {
			return err
		}
	}
	return nil
}

func (r *WorkflowStageIntegrationPostgresRepository) RecordReceipt(ctx context.Context, c stageintegrationmodel.Connection, result stageintegrationmodel.CallbackResult) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	if err = lockIntegrationConnection(ctx, q, c); err != nil {
		return false, err
	}
	n, err := q.InsertWorkflowStageReceipt(ctx, sqlc.InsertWorkflowStageReceiptParams{WorkspaceID: c.WorkspaceID, ConnectionID: c.ID, DeliveryID: result.DeliveryID, ConnectionVersion: c.Version})
	if err != nil {
		return false, err
	}
	if n > 0 {
		if err = insertIntegrationEvents(ctx, q, c, result.Events); err != nil {
			return false, err
		}
	}
	return n == 0, tx.Commit(ctx)
}

func (r *WorkflowStageIntegrationPostgresRepository) RecordEvents(ctx context.Context, c stageintegrationmodel.Connection, events []stageintegrationmodel.ProviderEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	if err = lockIntegrationConnection(ctx, q, c); err != nil {
		return err
	}
	if err = insertIntegrationEvents(ctx, q, c, events); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *WorkflowStageIntegrationPostgresRepository) Events(ctx context.Context, c stage.Candidate, obs stage.Observation) ([]stageintegrationmodel.ProviderEvent, error) {
	subjects, err := json.Marshal(obs.Subjects)
	if err != nil {
		return nil, err
	}
	rows, err := r.queries.ListWorkflowStageProviderEvents(ctx, sqlc.ListWorkflowStageProviderEventsParams{ConnectionID: c.Connection.ID, ActionID: c.ActionID, ActivatedAt: pgtype.Timestamptz{Time: obs.ActivatedAt, Valid: true}, ObservationID: obs.ID, Subjects: subjects})
	if err != nil {
		return nil, err
	}
	events := make([]stageintegrationmodel.ProviderEvent, 0, len(rows))
	for _, row := range rows {
		event := stageintegrationmodel.ProviderEvent{ID: row.SemanticID, ActionID: row.ActionID, OccurredAt: row.OccurredAt.Time, Facts: row.Facts}
		if err = json.Unmarshal(row.Subject, &event.Subject); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (r *WorkflowStageIntegrationPostgresRepository) AdmitEvent(ctx context.Context, c stage.Candidate, obs stage.Observation, event stageintegrationmodel.ProviderEvent, promptContext string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := sqlc.New(tx)
	if err = lockIntegrationCandidate(ctx, q, c); err != nil {
		return err
	}
	stored, err := q.WorkflowStageAdmissionObservation(ctx, sqlc.WorkflowStageAdmissionObservationParams{WorkspaceID: c.Connection.WorkspaceID, ID: obs.ID})
	if err != nil {
		return err
	}
	if stored.ID != c.ObservationID || stored.ConnectionVersion != c.Connection.Version || stored.ConfigurationVersion != c.ConfigurationVersion || !event.OccurredAt.After(stored.ActivatedAt.Time) {
		return problem.ErrConflict
	}
	exists, err := q.WorkflowStageAdmissionExists(ctx, sqlc.WorkflowStageAdmissionExistsParams{ObservationID: obs.ID, SemanticID: event.ID})
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	id := stableIntegrationID(obs.ID, event.ID)
	payload, err := json.Marshal(workflowmodel.IntegrationEvent{RuleID: c.RuleID, ConnectionID: c.Connection.ID, ConfigurationVersion: c.ConfigurationVersion, ConnectionVersion: c.Connection.Version, PromptContext: promptContext})
	if err != nil {
		return err
	}
	if err = q.EnqueueWorkflowIntegrationEvent(ctx, sqlc.EnqueueWorkflowIntegrationEventParams{ID: id, WorkspaceID: c.Connection.WorkspaceID, TaskID: c.TaskID, StageVersion: c.StageVersion, ActorID: c.ActorID, CausationID: event.ID, CorrelationID: obs.ID, Integration: payload, OccurredAt: pgtype.Timestamptz{Time: event.OccurredAt, Valid: true}}); err != nil {
		return err
	}
	if err = q.InsertWorkflowStageAdmission(ctx, sqlc.InsertWorkflowStageAdmissionParams{ObservationID: obs.ID, SemanticID: event.ID, EventID: id}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *WorkflowStageIntegrationPostgresRepository) ListStatus(ctx context.Context, workspaceID, connectionID string, position stageintegrationmodel.StatusPosition) ([]stageintegrationmodel.Status, error) {
	q := r.queries
	observations, err := q.ListWorkflowStageObservations(ctx, sqlc.ListWorkflowStageObservationsParams{WorkspaceID: workspaceID, ConnectionID: connectionID, HasCursor: position.ID != "", BeforeTime: pgtype.Timestamptz{Time: position.CreatedAt, Valid: true}, BeforeID: position.ID, PageSize: stageintegrationmodel.StatusPageSize + 1})
	if err != nil {
		return nil, err
	}
	out := make([]stageintegrationmodel.Status, 0, len(observations))
	for _, obs := range observations {
		state := "observing"
		if obs.LastError != "" {
			state = "failed"
		}
		out = append(out, stageintegrationmodel.Status{ID: obs.ID, TaskID: obs.TaskID, RuleID: obs.RuleID, StageVersion: obs.StageVersion, ActionID: obs.ActionID, State: state, Error: obs.LastError, CreatedAt: obs.CreatedAt.Time})
	}
	effects, err := q.ListWorkflowStageEffectStatus(ctx, sqlc.ListWorkflowStageEffectStatusParams{WorkspaceID: workspaceID, ConnectionID: connectionID, HasCursor: position.ID != "", BeforeTime: pgtype.Timestamptz{Time: position.CreatedAt, Valid: true}, BeforeID: position.ID, PageSize: stageintegrationmodel.StatusPageSize + 1})
	if err != nil {
		return nil, err
	}
	for _, effect := range effects {
		var event workflowmodel.IntegrationEvent
		if err = json.Unmarshal(effect.Integration, &event); err != nil {
			return nil, err
		}
		state := "pending"
		if effect.ProcessedAt.Valid {
			state = "processed"
		}
		if effect.ProcessingError != "" {
			state = "blocked"
		}
		diagnostic := effect.ProcessingError
		if effect.CommandState != "" {
			state = effect.CommandState
		}
		if effect.DeliveryError != "" {
			diagnostic = effect.DeliveryError
		}
		out = append(out, stageintegrationmodel.Status{ID: effect.ID, TaskID: effect.TaskID, RuleID: event.RuleID, StageVersion: effect.StageVersion, ActionID: effect.ActionID, State: state, Error: diagnostic, CreatedAt: effect.OccurredAt.Time})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if len(out) > stageintegrationmodel.StatusPageSize+1 {
		out = out[:stageintegrationmodel.StatusPageSize+1]
	}
	return out, nil
}

func (r *WorkflowStageIntegrationPostgresRepository) Retry(ctx context.Context, workspaceID, eventID string) error {
	n, err := r.queries.RetryWorkflowStageEvent(ctx, sqlc.RetryWorkflowStageEventParams{WorkspaceID: workspaceID, ID: eventID})
	if err != nil {
		return err
	}
	if n != 1 {
		return problem.ErrConflict
	}
	return nil
}
