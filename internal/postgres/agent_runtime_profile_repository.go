package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/josemiguelmelo/orkest/internal/agent"
	"github.com/josemiguelmelo/orkest/internal/agentruntimeprofile"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
)

type AgentRuntimeProfilePostgresRepository struct{ database *PostgresDatabase }

func NewAgentRuntimeProfilePostgresRepository(database *PostgresDatabase) *AgentRuntimeProfilePostgresRepository {
	return &AgentRuntimeProfilePostgresRepository{database: database}
}

var _ agentruntimeprofile.Repository = (*AgentRuntimeProfilePostgresRepository)(nil)

func scanAgentRuntimeProfile(row pgx.Row) (agentruntimeprofile.Profile, error) {
	var p agentruntimeprofile.Profile
	var runtime string
	err := row.Scan(&p.ID, &p.WorkspaceID, &p.Name, &runtime, &p.ModelProvider, &p.ModelIdentifier, &p.CreatedAt, &p.UpdatedAt)
	p.Runtime = agent.Runtime(runtime)
	return p, err
}

const agentRuntimeProfileColumns = `id, workspace_id, name, runtime, model_provider, model_identifier, created_at, updated_at`

func (r *AgentRuntimeProfilePostgresRepository) List(ctx context.Context, workspaceID string) ([]agentruntimeprofile.Profile, error) {
	rows, err := r.database.pool.Query(ctx, `SELECT `+agentRuntimeProfileColumns+` FROM agent_runtime_profiles WHERE workspace_id=$1 ORDER BY name,id`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles := make([]agentruntimeprofile.Profile, 0)
	for rows.Next() {
		p, scanErr := scanAgentRuntimeProfile(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

func (r *AgentRuntimeProfilePostgresRepository) Get(ctx context.Context, workspaceID, id string) (agentruntimeprofile.Profile, error) {
	p, err := scanAgentRuntimeProfile(r.database.pool.QueryRow(ctx, `SELECT `+agentRuntimeProfileColumns+` FROM agent_runtime_profiles WHERE workspace_id=$1 AND id=$2`, workspaceID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return agentruntimeprofile.Profile{}, modelerrors.ErrNotFound
	}
	return p, err
}

func (r *AgentRuntimeProfilePostgresRepository) Save(ctx context.Context, p agentruntimeprofile.Profile, create bool) (agentruntimeprofile.Profile, error) {
	tx, err := r.database.pool.Begin(ctx)
	if err != nil {
		return agentruntimeprofile.Profile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	query := `INSERT INTO agent_runtime_profiles (id,workspace_id,name,runtime,model_provider,model_identifier) VALUES($1,$2,$3,$4,$5,$6) RETURNING ` + agentRuntimeProfileColumns
	args := []any{p.ID, p.WorkspaceID, p.Name, string(p.Runtime), p.ModelProvider, p.ModelIdentifier}
	if !create {
		query = `UPDATE agent_runtime_profiles SET name=$3,runtime=$4,model_provider=$5,model_identifier=$6,updated_at=now() WHERE workspace_id=$2 AND id=$1 RETURNING ` + agentRuntimeProfileColumns
	}
	updated, err := scanAgentRuntimeProfile(tx.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return agentruntimeprofile.Profile{}, modelerrors.ErrNotFound
	}
	if err != nil {
		return agentruntimeprofile.Profile{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	var incompatible bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM agents a JOIN execution_profiles ep
		ON ep.workspace_id=a.workspace_id AND ep.id=a.execution_profile_id
		WHERE a.workspace_id=$1 AND a.runtime_profile_id=$2 AND ep.runtime<>$3
	)`, p.WorkspaceID, p.ID, string(p.Runtime)).Scan(&incompatible); err != nil {
		return agentruntimeprofile.Profile{}, err
	}
	if incompatible {
		return agentruntimeprofile.Profile{}, modelerrors.ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE agents SET runtime=$3,provider=$4,model=$5,updated_at=now() WHERE workspace_id=$1 AND runtime_profile_id=$2`, p.WorkspaceID, p.ID, string(p.Runtime), p.ModelProvider, p.ModelIdentifier); err != nil {
		return agentruntimeprofile.Profile{}, fmt.Errorf("apply runtime profile to agents: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return agentruntimeprofile.Profile{}, err
	}
	return updated, nil
}

func (r *AgentRuntimeProfilePostgresRepository) Delete(ctx context.Context, workspaceID, id string) error {
	result, err := r.database.pool.Exec(ctx, `DELETE FROM agent_runtime_profiles WHERE workspace_id=$1 AND id=$2`, workspaceID, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return modelerrors.ErrNotFound
	}
	return nil
}
