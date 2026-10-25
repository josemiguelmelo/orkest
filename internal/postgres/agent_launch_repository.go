package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/josemiguelmelo/orkest/internal/orchestration"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
)

type AgentLaunchPostgresRepository struct{ queries *sqlc.Queries }

func NewAgentLaunchPostgresRepository(database *PostgresDatabase) *AgentLaunchPostgresRepository {
	return &AgentLaunchPostgresRepository{queries: sqlc.New(database.pool)}
}

var _ orchestration.LaunchRepository = (*AgentLaunchPostgresRepository)(nil)

func (r *AgentLaunchPostgresRepository) CreateLaunch(ctx context.Context, launch orchestration.Launch) error {
	return r.queries.InsertAgentLaunch(ctx, sqlc.InsertAgentLaunchParams{RunID: launch.RunID, OrganizationID: launch.OrganizationID, WorkspaceID: launch.WorkspaceID, TaskID: launch.TaskID, Status: string(launch.Status)})
}

func (r *AgentLaunchPostgresRepository) UpdateLaunch(ctx context.Context, launch orchestration.Launch) error {
	return r.queries.UpdateAgentLaunch(ctx, sqlc.UpdateAgentLaunchParams{RunID: launch.RunID, Status: string(launch.Status), ProcessID: launch.ProcessID, Endpoint: launch.Endpoint, FailureStage: launch.FailureStage, FailureReason: launch.FailureReason})
}

func (r *AgentLaunchPostgresRepository) SaveEvent(ctx context.Context, event orchestration.Event) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("encode agent event: %w", err)
	}
	_, err = r.queries.SaveAgentLaunchEvent(ctx, sqlc.SaveAgentLaunchEventParams{RunID: event.RunID, LastSequence: int64(event.Sequence), Event: payload})
	return err
}
