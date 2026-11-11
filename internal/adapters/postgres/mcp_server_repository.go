package postgres

import (
	"context"

	"github.com/josemiguelmelo/orkest/internal/adapters/postgres/sqlc"
	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/server/mcpserver"
	mcpservermodel "github.com/josemiguelmelo/orkest/internal/server/mcpserver/model"
)

type MCPServerPostgresRepository struct{ database *PostgresDatabase }

func NewMCPServerPostgresRepository(database *PostgresDatabase) *MCPServerPostgresRepository {
	return &MCPServerPostgresRepository{database: database}
}

var _ mcpserver.MCPServerRepository = (*MCPServerPostgresRepository)(nil)

func mcpCredentials(server mcpservermodel.Server, encrypted string) (string, string) {
	if server.Transport == "stdio" {
		return "", encrypted
	}
	return encrypted, ""
}

func (r *MCPServerPostgresRepository) Create(ctx context.Context, server mcpservermodel.Server, encrypted string) (mcpservermodel.Server, error) {
	headers, environment := mcpCredentials(server, encrypted)
	row, err := sqlc.New(r.database.pool).CreateMCPServer(ctx, sqlc.CreateMCPServerParams{ID: server.ID, WorkspaceID: server.WorkspaceID, Name: server.Name, Url: server.URL, Transport: server.Transport, Command: server.Command, Args: append([]string{}, server.Args...), EncryptedHeaders: headers, EncryptedEnvironment: environment})
	if err != nil {
		return mcpservermodel.Server{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	return mcpServerFromRow(row), nil
}

func (r *MCPServerPostgresRepository) Update(ctx context.Context, server mcpservermodel.Server, encrypted string) (mcpservermodel.Server, error) {
	headers, environment := mcpCredentials(server, encrypted)
	row, err := sqlc.New(r.database.pool).UpdateMCPServer(ctx, sqlc.UpdateMCPServerParams{ID: server.ID, WorkspaceID: server.WorkspaceID, Name: server.Name, Url: server.URL, Transport: server.Transport, Command: server.Command, Args: append([]string{}, server.Args...), EncryptedHeaders: headers, EncryptedEnvironment: environment})
	if err != nil {
		return mcpservermodel.Server{}, mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	return mcpServerFromRow(row), nil
}

func (r *MCPServerPostgresRepository) Delete(ctx context.Context, workspaceID, id string) error {
	count, err := sqlc.New(r.database.pool).DeleteMCPServer(ctx, sqlc.DeleteMCPServerParams{WorkspaceID: workspaceID, ID: id})
	if err == nil && count == 0 {
		return modelerrors.ErrNotFound
	}
	return err
}

func (r *MCPServerPostgresRepository) Get(ctx context.Context, workspaceID, id string) (mcpservermodel.Server, string, error) {
	row, err := sqlc.New(r.database.pool).GetMCPServer(ctx, sqlc.GetMCPServerParams{WorkspaceID: workspaceID, ID: id})
	if err != nil {
		return mcpservermodel.Server{}, "", mapError(err, modelerrors.ErrNotFound, modelerrors.ErrConflict)
	}
	secret := row.EncryptedHeaders
	if row.Transport == "stdio" {
		secret = row.EncryptedEnvironment
	}
	return mcpServerFromRow(row), secret, nil
}

func (r *MCPServerPostgresRepository) List(ctx context.Context, workspaceID string) ([]mcpservermodel.Server, error) {
	rows, err := sqlc.New(r.database.pool).ListMCPServers(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	servers := make([]mcpservermodel.Server, 0, len(rows))
	for _, row := range rows {
		servers = append(servers, mcpServerFromRow(row))
	}
	return servers, nil
}

func mcpServerFromRow(row sqlc.McpServer) mcpservermodel.Server {
	return mcpservermodel.Server{ID: row.ID, WorkspaceID: row.WorkspaceID, Name: row.Name, Transport: row.Transport, URL: row.Url, Command: row.Command, Args: row.Args, HasHeaders: row.EncryptedHeaders != "", HasEnvironment: row.EncryptedEnvironment != ""}
}
