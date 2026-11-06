package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	"github.com/josemiguelmelo/orkest/internal/platform/secretbox"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

type AgentLaunchPostgresRepository struct {
	queries   *sqlc.Queries
	pool      *pgxpool.Pool
	secretBox *secretbox.Box
}

func NewAgentLaunchPostgresRepository(database *PostgresDatabase, box *secretbox.Box) *AgentLaunchPostgresRepository {
	repo := &AgentLaunchPostgresRepository{queries: sqlc.New(database.pool), pool: database.pool}
	repo.secretBox = box
	return repo
}

var (
	_ orchestration.LaunchRepository = (*AgentLaunchPostgresRepository)(nil)
	_ orchestration.TaskLaunchReader = (*AgentLaunchPostgresRepository)(nil)
)

func (r *AgentLaunchPostgresRepository) CreateLaunch(ctx context.Context, launch orchestrationmodel.Launch) error {
	return r.queries.InsertAgentLaunch(ctx, sqlc.InsertAgentLaunchParams{ExecutionImage: launch.ExecutionImage, RunID: launch.RunID, OrganizationID: launch.OrganizationID, WorkspaceID: launch.WorkspaceID, TaskID: launch.TaskID, Status: string(launch.Status), TaskSessionID: pgtype.Text{String: launch.TaskSessionID, Valid: launch.TaskSessionID != ""}})
}

func (r *AgentLaunchPostgresRepository) UpdateLaunch(ctx context.Context, launch orchestrationmodel.Launch) error {
	return r.queries.UpdateAgentLaunch(ctx, sqlc.UpdateAgentLaunchParams{RunID: launch.RunID, Status: string(launch.Status), ProcessID: launch.ProcessID, Endpoint: launch.Endpoint, FailureStage: launch.FailureStage, FailureReason: launch.FailureReason})
}

func (r *AgentLaunchPostgresRepository) ListReconnectableLaunches(ctx context.Context) ([]orchestrationmodel.Launch, error) {
	rows, err := r.queries.ListReconnectableAgentLaunches(ctx)
	if err != nil {
		return nil, fmt.Errorf("list reconnectable agent launches: %w", err)
	}
	launches := make([]orchestrationmodel.Launch, 0, len(rows))
	for _, row := range rows {
		launches = append(launches, orchestrationmodel.Launch{
			ExecutionImage: row.ExecutionImage, RunID: row.RunID,
			TaskSessionID:  row.TaskSessionID.String,
			OrganizationID: row.OrganizationID,
			WorkspaceID:    row.WorkspaceID,
			TaskID:         row.TaskID,
			Status:         orchestrationmodel.LaunchStatus(row.Status),
			ProcessID:      row.ProcessID,
			Endpoint:       row.Endpoint,
			FailureStage:   row.FailureStage,
			FailureReason:  row.FailureReason,
			LastSequence:   uint64(row.LastSequence),
			LastEventType:  row.LastEventType,
			UpdatedAt:      row.UpdatedAt.Time,
		})
	}
	return launches, nil
}

func (r *AgentLaunchPostgresRepository) SaveEvent(ctx context.Context, event orchestrationmodel.Event) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("encode agent event: %w", err)
	}
	if event.Payload.Type == orchestrationmodel.EventPromptAccepted && event.Payload.CommandID != "" {
		if err := r.PromptDelivered(ctx, event.RunID, event.Payload.CommandID); err != nil {
			return err
		}
	}
	if event.Payload.TaskSessionID != "" && event.Payload.SessionID != "" {
		if err := r.queries.SetRuntimeSessionID(ctx, sqlc.SetRuntimeSessionIDParams{ID: event.Payload.TaskSessionID, RuntimeSessionID: event.Payload.SessionID}); err != nil {
			return err
		}
	}
	_, err = r.queries.SaveAgentLaunchEvent(ctx, sqlc.SaveAgentLaunchEventParams{RunID: event.RunID, LastSequence: int64(event.Sequence), Event: payload})
	return err
}

func (r *AgentLaunchPostgresRepository) SaveTerminalEvent(ctx context.Context, event orchestrationmodel.Event, status orchestrationmodel.LaunchStatus) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("encode terminal agent event: %w", err)
	}
	return r.queries.SaveTerminalAgentLaunchEvent(ctx, sqlc.SaveTerminalAgentLaunchEventParams{
		RunID: event.RunID, LastSequence: int64(event.Sequence), Event: payload, Status: string(status),
	})
}

func (r *AgentLaunchPostgresRepository) SetLaunchStatus(ctx context.Context, effects orchestration.LaunchStatusEffects) error {
	runID, status := effects.RunID, effects.Status
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	count, err := q.SetAgentLaunchStatus(ctx, sqlc.SetAgentLaunchStatusParams{RunID: runID, Status: string(status)})
	if err != nil {
		return err
	}
	// A stale terminal callback must not emit an outcome for a rejected transition.
	if count == 0 {
		return tx.Commit(ctx)
	}
	if effects.Outcome != nil {
		run, err := q.RunWorkflowEntry(ctx, runID)
		if err != nil {
			return err
		}
		event := *effects.Outcome
		event.StageVersion = run.StageVersion
		if err = persistStageEvent(ctx, q, run.WorkspaceID, run.TaskID, run.ActorID.String, event); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *AgentLaunchPostgresRepository) ListTaskLaunches(ctx context.Context, orgID, workspaceID, taskID string) ([]orchestrationmodel.Launch, error) {
	rows, err := r.queries.ListTaskAgentLaunches(ctx, sqlc.ListTaskAgentLaunchesParams{OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: taskID})
	if err != nil {
		return nil, fmt.Errorf("list task agent launches: %w", err)
	}
	items := make([]orchestrationmodel.Launch, 0, len(rows))
	for _, row := range rows {
		items = append(items, orchestrationmodel.Launch{ExecutionImage: row.ExecutionImage, RunID: row.RunID, TaskSessionID: row.TaskSessionID.String, OrganizationID: row.OrganizationID, WorkspaceID: row.WorkspaceID, TaskID: row.TaskID, Status: orchestrationmodel.LaunchStatus(row.Status), Endpoint: row.Endpoint, FailureStage: row.FailureStage, FailureReason: row.FailureReason, LastSequence: uint64(row.LastSequence), UpdatedAt: row.UpdatedAt.Time})
	}
	return items, nil
}

func (r *AgentLaunchPostgresRepository) ListTaskLaunchEvents(ctx context.Context, runID string, after uint64) ([]orchestrationmodel.Event, error) {
	rows, err := r.queries.ListTaskAgentLaunchEvents(ctx, sqlc.ListTaskAgentLaunchEventsParams{RunID: runID, Sequence: int64(after)})
	if err != nil {
		return nil, fmt.Errorf("list task agent events: %w", err)
	}
	items := make([]orchestrationmodel.Event, 0, len(rows))
	for _, row := range rows {
		var payload orchestrationmodel.AgentActivityEvent
		if err := json.Unmarshal(row.Event, &payload); err != nil {
			return nil, fmt.Errorf("decode task agent event: %w", err)
		}
		items = append(items, orchestrationmodel.Event{RunID: runID, Sequence: uint64(row.Sequence), Payload: payload})
	}
	return items, nil
}
