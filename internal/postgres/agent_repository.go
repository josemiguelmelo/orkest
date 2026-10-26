package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/josemiguelmelo/orkest/internal/agent"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/postgres/sqlc"
)

type AgentPostgresRepository struct {
	queries  *sqlc.Queries
	database *PostgresDatabase
}

func NewAgentPostgresRepository(database *PostgresDatabase) *AgentPostgresRepository {
	return &AgentPostgresRepository{queries: sqlc.New(database.pool), database: database}
}

var _ agent.Repository = (*AgentPostgresRepository)(nil)

func (r *AgentPostgresRepository) Agents(ctx context.Context, orgID, workspaceID string) ([]agent.Agent, error) {
	rows, err := r.queries.ListAgents(ctx, sqlc.ListAgentsParams{OrganizationID: orgID, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	out := make([]agent.Agent, 0, len(rows))
	for _, row := range rows {
		value := agent.Agent{ID: row.ID, WorkspaceID: row.WorkspaceID, AgentDefinition: agent.AgentDefinition{Name: row.Name, Runtime: agent.Runtime(row.Runtime), Provider: row.Provider, Model: row.Model, Description: row.Description, SystemPrompt: row.SystemPrompt}}
		value.MCPServerIDs, err = r.mcpServerIDs(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, nil
}

func (r *AgentPostgresRepository) Agent(ctx context.Context, orgID, workspaceID, id string) (agent.Agent, error) {
	row, err := r.queries.GetAgent(ctx, sqlc.GetAgentParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id})
	a := agent.Agent{ID: row.ID, WorkspaceID: row.WorkspaceID, AgentDefinition: agent.AgentDefinition{Name: row.Name, Runtime: agent.Runtime(row.Runtime), Provider: row.Provider, Model: row.Model, Description: row.Description, SystemPrompt: row.SystemPrompt}}
	if err != nil {
		return agent.Agent{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	a.MCPServerIDs, err = r.mcpServerIDs(ctx, id)
	return a, err
}

func (r *AgentPostgresRepository) CreateAgent(ctx context.Context, id, workspaceID string, definition agent.AgentDefinition) (agent.Agent, error) {
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return agent.Agent{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := r.queries.WithTx(tx)
	row, err := queries.InsertAgent(ctx, sqlc.InsertAgentParams{ID: id, ID_2: workspaceID, Name: definition.Name, Runtime: string(definition.Runtime), Provider: definition.Provider, Model: definition.Model, Description: definition.Description, SystemPrompt: definition.SystemPrompt})
	if err == nil {
		err = replaceAgentMCPServers(ctx, tx, workspaceID, id, definition.MCPServerIDs)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	a := agent.Agent{ID: row.ID, WorkspaceID: row.WorkspaceID, AgentDefinition: agent.AgentDefinition{Name: row.Name, Runtime: agent.Runtime(row.Runtime), Provider: row.Provider, Model: row.Model, Description: row.Description, SystemPrompt: row.SystemPrompt}}
	a.MCPServerIDs = definition.MCPServerIDs
	return a, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (r *AgentPostgresRepository) UpdateAgent(ctx context.Context, orgID, workspaceID, id string, definition agent.AgentDefinition) (agent.Agent, error) {
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return agent.Agent{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := r.queries.WithTx(tx)
	row, err := queries.UpdateAgent(ctx, sqlc.UpdateAgentParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id, Name: definition.Name, Runtime: string(definition.Runtime), Provider: definition.Provider, Model: definition.Model, Description: definition.Description, SystemPrompt: definition.SystemPrompt})
	if err == nil {
		err = replaceAgentMCPServers(ctx, tx, workspaceID, id, definition.MCPServerIDs)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	a := agent.Agent{ID: row.ID, WorkspaceID: row.WorkspaceID, AgentDefinition: agent.AgentDefinition{Name: row.Name, Runtime: agent.Runtime(row.Runtime), Provider: row.Provider, Model: row.Model, Description: row.Description, SystemPrompt: row.SystemPrompt}}
	a.MCPServerIDs = definition.MCPServerIDs
	return a, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (r *AgentPostgresRepository) DeleteAgent(ctx context.Context, orgID, workspaceID, id string) error {
	count, err := r.queries.DeleteAgent(ctx, sqlc.DeleteAgentParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id})
	if err == nil && count == 0 {
		return modelerrors.ErrNotFound
	}
	return err
}

func (r *AgentPostgresRepository) mcpServerIDs(ctx context.Context, agentID string) ([]string, error) {
	rows, err := r.database.pool.Query(ctx, `SELECT mcp_server_id FROM agent_mcp_servers WHERE agent_id=$1 ORDER BY mcp_server_id`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func replaceAgentMCPServers(ctx context.Context, tx pgx.Tx, workspaceID, agentID string, ids []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM agent_mcp_servers WHERE workspace_id=$1 AND agent_id=$2`, workspaceID, agentID); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.Exec(ctx, `INSERT INTO agent_mcp_servers(workspace_id,agent_id,mcp_server_id) VALUES($1,$2,$3)`, workspaceID, agentID, id); err != nil {
			return fmt.Errorf("save agent MCP server selection: %w", err)
		}
	}
	return nil
}
