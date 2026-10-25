package postgres

import (
	"context"

	"github.com/josemiguelmelo/orkest/internal/agent"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
)

type AgentPostgresRepository struct {
	queries *sqlc.Queries
}

func NewAgentPostgresRepository(database *PostgresDatabase) *AgentPostgresRepository {
	return &AgentPostgresRepository{queries: sqlc.New(database.pool)}
}

var _ agent.Repository = (*AgentPostgresRepository)(nil)

func (r *AgentPostgresRepository) Agents(ctx context.Context, orgID, workspaceID string) ([]agent.Agent, error) {
	rows, err := r.queries.ListAgents(ctx, sqlc.ListAgentsParams{OrganizationID: orgID, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	out := make([]agent.Agent, 0, len(rows))
	for _, row := range rows {
		out = append(out, agent.Agent{ID: row.ID, WorkspaceID: row.WorkspaceID, AgentDefinition: agent.AgentDefinition{Name: row.Name, Runtime: agent.Runtime(row.Runtime), Provider: row.Provider, Model: row.Model, Description: row.Description, SystemPrompt: row.SystemPrompt}})
	}
	return out, nil
}

func (r *AgentPostgresRepository) Agent(ctx context.Context, orgID, workspaceID, id string) (agent.Agent, error) {
	row, err := r.queries.GetAgent(ctx, sqlc.GetAgentParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id})
	a := agent.Agent{ID: row.ID, WorkspaceID: row.WorkspaceID, AgentDefinition: agent.AgentDefinition{Name: row.Name, Runtime: agent.Runtime(row.Runtime), Provider: row.Provider, Model: row.Model, Description: row.Description, SystemPrompt: row.SystemPrompt}}
	return a, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (r *AgentPostgresRepository) CreateAgent(ctx context.Context, id, workspaceID string, definition agent.AgentDefinition) (agent.Agent, error) {
	row, err := r.queries.InsertAgent(ctx, sqlc.InsertAgentParams{ID: id, ID_2: workspaceID, Name: definition.Name, Runtime: string(definition.Runtime), Provider: definition.Provider, Model: definition.Model, Description: definition.Description, SystemPrompt: definition.SystemPrompt})
	a := agent.Agent{ID: row.ID, WorkspaceID: row.WorkspaceID, AgentDefinition: agent.AgentDefinition{Name: row.Name, Runtime: agent.Runtime(row.Runtime), Provider: row.Provider, Model: row.Model, Description: row.Description, SystemPrompt: row.SystemPrompt}}
	return a, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (r *AgentPostgresRepository) UpdateAgent(ctx context.Context, orgID, workspaceID, id string, definition agent.AgentDefinition) (agent.Agent, error) {
	row, err := r.queries.UpdateAgent(ctx, sqlc.UpdateAgentParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id, Name: definition.Name, Runtime: string(definition.Runtime), Provider: definition.Provider, Model: definition.Model, Description: definition.Description, SystemPrompt: definition.SystemPrompt})
	a := agent.Agent{ID: row.ID, WorkspaceID: row.WorkspaceID, AgentDefinition: agent.AgentDefinition{Name: row.Name, Runtime: agent.Runtime(row.Runtime), Provider: row.Provider, Model: row.Model, Description: row.Description, SystemPrompt: row.SystemPrompt}}
	return a, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (r *AgentPostgresRepository) DeleteAgent(ctx context.Context, orgID, workspaceID, id string) error {
	count, err := r.queries.DeleteAgent(ctx, sqlc.DeleteAgentParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id})
	if err == nil && count == 0 {
		return modelerrors.ErrNotFound
	}
	return err
}
