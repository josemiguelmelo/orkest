package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/secretbox"
	"github.com/josemiguelmelo/orkest/internal/server/agent"
	agentmodel "github.com/josemiguelmelo/orkest/internal/server/agent/model"
)

type AgentPostgresRepository struct {
	queries   *sqlc.Queries
	database  *PostgresDatabase
	secretBox *secretbox.Box
}

func NewAgentPostgresRepository(database *PostgresDatabase, box *secretbox.Box) *AgentPostgresRepository {
	return &AgentPostgresRepository{secretBox: box, queries: sqlc.New(database.pool), database: database}
}

var _ agent.AgentDefinitionRepository = (*AgentPostgresRepository)(nil)

func (r *AgentPostgresRepository) Agents(ctx context.Context, orgID, workspaceID string) ([]agentmodel.Agent, error) {
	rows, err := r.queries.ListAgents(ctx, sqlc.ListAgentsParams{OrganizationID: orgID, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	out := make([]agentmodel.Agent, 0, len(rows))
	for _, row := range rows {
		value := agentmodel.Agent{ID: row.ID, WorkspaceID: row.WorkspaceID, AgentDefinition: agentmodel.AgentDefinition{Name: row.Name, Runtime: agentmodel.Runtime(row.Runtime), Provider: row.Provider, Model: row.Model, Description: row.Description, ExecutionProfileID: row.ExecutionProfileID.String, RuntimeProfileID: row.RuntimeProfileID.String, SystemPrompt: row.SystemPrompt, PermissionPolicy: agentmodel.PermissionPolicy(row.PermissionPolicy)}}
		value.MCPServerIDs, err = r.mcpServerIDs(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		value.SkillIDs, err = r.skillIDs(ctx, row.WorkspaceID, row.ID)
		if err != nil {
			return nil, err
		}
		value.Configuration, err = r.configuration(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		value.HasConfiguration = len(value.Configuration) > 0
		out = append(out, value)
	}
	return out, nil
}

func (r *AgentPostgresRepository) Agent(ctx context.Context, orgID, workspaceID, id string) (agentmodel.Agent, error) {
	row, err := r.queries.GetAgent(ctx, sqlc.GetAgentParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return agentmodel.Agent{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	a := agentmodel.Agent{ID: row.ID, WorkspaceID: row.WorkspaceID, AgentDefinition: agentmodel.AgentDefinition{Name: row.Name, Runtime: agentmodel.Runtime(row.Runtime), Provider: row.Provider, Model: row.Model, Description: row.Description, ExecutionProfileID: row.ExecutionProfileID.String, RuntimeProfileID: row.RuntimeProfileID.String, SystemPrompt: row.SystemPrompt, PermissionPolicy: agentmodel.PermissionPolicy(row.PermissionPolicy)}}
	a.Configuration, err = r.configuration(ctx, id)
	if err != nil {
		return agentmodel.Agent{}, err
	}
	a.HasConfiguration = len(a.Configuration) > 0
	a.MCPServerIDs, err = r.mcpServerIDs(ctx, id)
	if err == nil {
		a.SkillIDs, err = r.skillIDs(ctx, workspaceID, id)
	}
	return a, err
}

func (r *AgentPostgresRepository) CreateAgent(ctx context.Context, id, workspaceID string, definition agentmodel.AgentDefinition) (agentmodel.Agent, error) {
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return agentmodel.Agent{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := r.queries.WithTx(tx)
	row, err := queries.InsertAgent(ctx, sqlc.InsertAgentParams{ID: id, ID_2: workspaceID, Name: definition.Name, Runtime: string(definition.Runtime), Provider: definition.Provider, Model: definition.Model, Description: definition.Description, ExecutionProfileID: pgtype.Text{String: definition.ExecutionProfileID, Valid: definition.ExecutionProfileID != ""}, RuntimeProfileID: pgtype.Text{String: definition.RuntimeProfileID, Valid: definition.RuntimeProfileID != ""}, SystemPrompt: definition.SystemPrompt, PermissionPolicy: string(definition.PermissionPolicy)})
	if err == nil {
		var encrypted []byte
		encrypted, err = secretbox.SealConfiguration(r.secretBox, definition.Configuration)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE agents SET runtime_configuration=$3 WHERE workspace_id=$1 AND id=$2`, workspaceID, id, encrypted)
		}
	}
	if err == nil {
		err = replaceAgentMCPServers(ctx, tx, workspaceID, id, definition.MCPServerIDs)
	}
	if err == nil {
		err = replaceAgentSkills(ctx, queries, workspaceID, id, definition.SkillIDs)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	a := agentmodel.Agent{ID: row.ID, WorkspaceID: row.WorkspaceID, AgentDefinition: agentmodel.AgentDefinition{Name: row.Name, Runtime: agentmodel.Runtime(row.Runtime), Provider: row.Provider, Model: row.Model, Description: row.Description, ExecutionProfileID: row.ExecutionProfileID.String, RuntimeProfileID: row.RuntimeProfileID.String, SystemPrompt: row.SystemPrompt, PermissionPolicy: definition.PermissionPolicy}}
	a.Configuration = definition.Configuration
	a.HasConfiguration = len(a.Configuration) > 0
	a.MCPServerIDs = definition.MCPServerIDs
	a.SkillIDs = definition.SkillIDs
	return a, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (r *AgentPostgresRepository) UpdateAgent(ctx context.Context, orgID, workspaceID, id string, definition agentmodel.AgentDefinition) (agentmodel.Agent, error) {
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return agentmodel.Agent{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := r.queries.WithTx(tx)
	row, err := queries.UpdateAgent(ctx, sqlc.UpdateAgentParams{OrganizationID: orgID, WorkspaceID: workspaceID, ID: id, Name: definition.Name, Runtime: string(definition.Runtime), Provider: definition.Provider, Model: definition.Model, Description: definition.Description, ExecutionProfileID: pgtype.Text{String: definition.ExecutionProfileID, Valid: definition.ExecutionProfileID != ""}, RuntimeProfileID: pgtype.Text{String: definition.RuntimeProfileID, Valid: definition.RuntimeProfileID != ""}, SystemPrompt: definition.SystemPrompt, PermissionPolicy: string(definition.PermissionPolicy)})
	if err == nil {
		var encrypted []byte
		encrypted, err = secretbox.SealConfiguration(r.secretBox, definition.Configuration)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE agents SET runtime_configuration=$3 WHERE workspace_id=$1 AND id=$2`, workspaceID, id, encrypted)
		}
	}
	if err == nil {
		err = replaceAgentMCPServers(ctx, tx, workspaceID, id, definition.MCPServerIDs)
	}
	if err == nil {
		err = replaceAgentSkills(ctx, queries, workspaceID, id, definition.SkillIDs)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	a := agentmodel.Agent{ID: row.ID, WorkspaceID: row.WorkspaceID, AgentDefinition: agentmodel.AgentDefinition{Name: row.Name, Runtime: agentmodel.Runtime(row.Runtime), Provider: row.Provider, Model: row.Model, Description: row.Description, ExecutionProfileID: row.ExecutionProfileID.String, RuntimeProfileID: row.RuntimeProfileID.String, SystemPrompt: row.SystemPrompt, PermissionPolicy: agentmodel.PermissionPolicy(row.PermissionPolicy)}}
	a.Configuration = definition.Configuration
	a.HasConfiguration = len(a.Configuration) > 0
	a.MCPServerIDs = definition.MCPServerIDs
	a.SkillIDs = definition.SkillIDs
	return a, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
}

func (r *AgentPostgresRepository) skillIDs(ctx context.Context, workspaceID, agentID string) ([]string, error) {
	rows, err := r.queries.ListAgentSkillIDs(ctx, sqlc.ListAgentSkillIDsParams{WorkspaceID: workspaceID, AgentID: agentID})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	ids = append(ids, rows...)
	return ids, nil
}

func (r *AgentPostgresRepository) configuration(ctx context.Context, id string) (map[string]string, error) {
	var encrypted []byte
	if err := r.database.pool.QueryRow(ctx, `SELECT runtime_configuration FROM agents WHERE id=$1`, id).Scan(&encrypted); err != nil {
		return nil, err
	}
	return secretbox.OpenConfiguration(r.secretBox, encrypted)
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

func replaceAgentSkills(ctx context.Context, queries *sqlc.Queries, workspaceID, agentID string, ids []string) error {
	if err := queries.DeleteAgentSkills(ctx, sqlc.DeleteAgentSkillsParams{WorkspaceID: workspaceID, AgentID: agentID}); err != nil {
		return err
	}
	for _, id := range ids {
		if err := queries.InsertAgentSkill(ctx, sqlc.InsertAgentSkillParams{WorkspaceID: workspaceID, AgentID: agentID, SkillID: id}); err != nil {
			return fmt.Errorf("save agent skill selection: %w", err)
		}
	}
	return nil
}
