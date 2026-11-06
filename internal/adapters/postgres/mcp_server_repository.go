package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/mcpserver"
)

type MCPServerPostgresRepository struct{ database *PostgresDatabase }

func NewMCPServerPostgresRepository(database *PostgresDatabase) *MCPServerPostgresRepository {
	return &MCPServerPostgresRepository{database: database}
}

var _ mcpserver.MCPServerRepository = (*MCPServerPostgresRepository)(nil)

func (r *MCPServerPostgresRepository) Create(ctx context.Context, id, workspaceID, name, endpoint, encrypted string) (mcpserver.Server, error) {
	return scanMCPServer(r.database.pool.QueryRow(ctx, `INSERT INTO mcp_servers(id, workspace_id, name, url, encrypted_headers) VALUES($1,$2,$3,$4,$5) RETURNING id, workspace_id, name, url, encrypted_headers <> ''`, id, workspaceID, name, endpoint, encrypted))
}

func (r *MCPServerPostgresRepository) Update(ctx context.Context, workspaceID, id, name, endpoint, encrypted string) (mcpserver.Server, error) {
	server, err := scanMCPServer(r.database.pool.QueryRow(ctx, `UPDATE mcp_servers SET name=$3,url=$4,encrypted_headers=$5,updated_at=now() WHERE workspace_id=$1 AND id=$2 RETURNING id,workspace_id,name,url,encrypted_headers <> ''`, workspaceID, id, name, endpoint, encrypted))
	if err != nil {
		return mcpserver.Server{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	return server, nil
}

func (r *MCPServerPostgresRepository) Delete(ctx context.Context, workspaceID, id string) error {
	tag, err := r.database.pool.Exec(ctx, `DELETE FROM mcp_servers WHERE workspace_id=$1 AND id=$2`, workspaceID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return modelerrors.ErrNotFound
	}
	return err
}

func (r *MCPServerPostgresRepository) Get(ctx context.Context, workspaceID, id string) (mcpserver.Server, string, error) {
	var server mcpserver.Server
	var encrypted string
	err := r.database.pool.QueryRow(ctx, `SELECT id,workspace_id,name,url,encrypted_headers FROM mcp_servers WHERE workspace_id=$1 AND id=$2`, workspaceID, id).Scan(&server.ID, &server.WorkspaceID, &server.Name, &server.URL, &encrypted)
	if err != nil {
		return mcpserver.Server{}, "", mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	server.HasHeaders = encrypted != ""
	return server, encrypted, nil
}

func (r *MCPServerPostgresRepository) List(ctx context.Context, workspaceID string) ([]mcpserver.Server, error) {
	rows, err := r.database.pool.Query(ctx, `SELECT id,workspace_id,name,url,encrypted_headers <> '' FROM mcp_servers WHERE workspace_id=$1 ORDER BY name,id`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	servers := make([]mcpserver.Server, 0)
	for rows.Next() {
		var server mcpserver.Server
		if err = rows.Scan(&server.ID, &server.WorkspaceID, &server.Name, &server.URL, &server.HasHeaders); err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	return servers, rows.Err()
}

func scanMCPServer(row pgx.Row) (mcpserver.Server, error) {
	var server mcpserver.Server
	err := row.Scan(&server.ID, &server.WorkspaceID, &server.Name, &server.URL, &server.HasHeaders)
	if err != nil {
		return mcpserver.Server{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	return server, nil
}
