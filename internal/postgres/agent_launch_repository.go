package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	"github.com/josemiguelmelo/orkest/internal/core/secretbox"
	"github.com/josemiguelmelo/orkest/internal/orchestration"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
)

type AgentLaunchPostgresRepository struct {
	queries   *sqlc.Queries
	pool      *pgxpool.Pool
	secretBox *secretbox.Box
}

func NewAgentLaunchPostgresRepository(database *PostgresDatabase, boxes ...*secretbox.Box) *AgentLaunchPostgresRepository {
	repo := &AgentLaunchPostgresRepository{queries: sqlc.New(database.pool), pool: database.pool}
	if len(boxes) > 0 {
		repo.secretBox = boxes[0]
	}
	return repo
}

var (
	_ orchestration.LaunchRepository = (*AgentLaunchPostgresRepository)(nil)
	_ orchestration.TaskLaunchReader = (*AgentLaunchPostgresRepository)(nil)
)

func (r *AgentLaunchPostgresRepository) CreateLaunch(ctx context.Context, launch orchestration.Launch) error {
	return r.queries.InsertAgentLaunch(ctx, sqlc.InsertAgentLaunchParams{RunID: launch.RunID, OrganizationID: launch.OrganizationID, WorkspaceID: launch.WorkspaceID, TaskID: launch.TaskID, Status: string(launch.Status), TaskSessionID: pgtype.Text{String: launch.TaskSessionID, Valid: launch.TaskSessionID != ""}})
}

func (r *AgentLaunchPostgresRepository) UpdateLaunch(ctx context.Context, launch orchestration.Launch) error {
	return r.queries.UpdateAgentLaunch(ctx, sqlc.UpdateAgentLaunchParams{RunID: launch.RunID, Status: string(launch.Status), ProcessID: launch.ProcessID, Endpoint: launch.Endpoint, FailureStage: launch.FailureStage, FailureReason: launch.FailureReason})
}

func (r *AgentLaunchPostgresRepository) ListReconnectableLaunches(ctx context.Context) ([]orchestration.Launch, error) {
	rows, err := r.queries.ListReconnectableAgentLaunches(ctx)
	if err != nil {
		return nil, fmt.Errorf("list reconnectable agent launches: %w", err)
	}
	launches := make([]orchestration.Launch, 0, len(rows))
	for _, row := range rows {
		launches = append(launches, orchestration.Launch{
			RunID:          row.RunID,
			TaskSessionID:  row.TaskSessionID.String,
			OrganizationID: row.OrganizationID,
			WorkspaceID:    row.WorkspaceID,
			TaskID:         row.TaskID,
			Status:         orchestration.LaunchStatus(row.Status),
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

func (r *AgentLaunchPostgresRepository) SaveEvent(ctx context.Context, event orchestration.Event) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("encode agent event: %w", err)
	}
	if event.Payload.Type == interaction.EventPromptAccepted && event.Payload.CommandID != "" {
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

func (r *AgentLaunchPostgresRepository) SaveTerminalEvent(ctx context.Context, event orchestration.Event, status orchestration.LaunchStatus) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("encode terminal agent event: %w", err)
	}
	return r.queries.SaveTerminalAgentLaunchEvent(ctx, sqlc.SaveTerminalAgentLaunchEventParams{
		RunID: event.RunID, LastSequence: int64(event.Sequence), Event: payload, Status: string(status),
	})
}

func (r *AgentLaunchPostgresRepository) SetLaunchStatus(ctx context.Context, runID string, status orchestration.LaunchStatus) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.queries.WithTx(tx)
	if err := q.SetAgentLaunchStatus(ctx, sqlc.SetAgentLaunchStatusParams{RunID: runID, Status: string(status)}); err != nil {
		return err
	}
	if status == orchestration.LaunchCompleted {
		if err := q.CompleteTaskForRun(ctx, runID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *AgentLaunchPostgresRepository) ListTaskLaunches(ctx context.Context, orgID, workspaceID, taskID string) ([]orchestration.Launch, error) {
	rows, err := r.queries.ListTaskAgentLaunches(ctx, sqlc.ListTaskAgentLaunchesParams{OrganizationID: orgID, WorkspaceID: workspaceID, TaskID: taskID})
	if err != nil {
		return nil, fmt.Errorf("list task agent launches: %w", err)
	}
	items := make([]orchestration.Launch, 0, len(rows))
	for _, row := range rows {
		items = append(items, orchestration.Launch{RunID: row.RunID, TaskSessionID: row.TaskSessionID.String, OrganizationID: row.OrganizationID, WorkspaceID: row.WorkspaceID, TaskID: row.TaskID, Status: orchestration.LaunchStatus(row.Status), Endpoint: row.Endpoint, FailureStage: row.FailureStage, FailureReason: row.FailureReason, LastSequence: uint64(row.LastSequence), UpdatedAt: row.UpdatedAt.Time})
	}
	return items, nil
}

func (r *AgentLaunchPostgresRepository) ListTaskLaunchEvents(ctx context.Context, runID string, after uint64) ([]orchestration.Event, error) {
	rows, err := r.queries.ListTaskAgentLaunchEvents(ctx, sqlc.ListTaskAgentLaunchEventsParams{RunID: runID, Sequence: int64(after)})
	if err != nil {
		return nil, fmt.Errorf("list task agent events: %w", err)
	}
	items := make([]orchestration.Event, 0, len(rows))
	for _, row := range rows {
		var payload interaction.Event
		if err := json.Unmarshal(row.Event, &payload); err != nil {
			return nil, fmt.Errorf("decode task agent event: %w", err)
		}
		items = append(items, orchestration.Event{RunID: runID, Sequence: uint64(row.Sequence), Payload: payload})
	}
	return items, nil
}
