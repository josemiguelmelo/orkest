package postgres

import (
	"context"
	"errors"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	mcpservermodel "github.com/josemiguelmelo/orkest/internal/server/mcpserver/model"
)

func testMCPServers(t *testing.T, database *PostgresDatabase, workspaceID string) {
	t.Helper()
	ctx := context.Background()
	repo := NewMCPServerPostgresRepository(database)
	for _, transport := range []string{"http", "stdio"} {
		id, err := ids.NewUUID()
		if err != nil {
			t.Fatal(err)
		}
		server := mcpservermodel.Server{ID: id, WorkspaceID: workspaceID, Name: "MCP " + transport, Transport: transport}
		if transport == "stdio" {
			server.Command = "/opt/mcp/server"
			server.Args = []string{"literal space"}
		} else {
			server.URL = "https://example.test/mcp"
		}
		created, err := repo.Create(ctx, server, "encrypted-test-value")
		if err != nil {
			t.Fatal(err)
		}
		if created.HasHeaders != (transport == "http") || created.HasEnvironment != (transport == "stdio") {
			t.Fatal("credentials stored for wrong transport")
		}
		got, secret, err := repo.Get(ctx, workspaceID, id)
		if err != nil || secret != "encrypted-test-value" || got.Transport != transport || got.Command != server.Command {
			t.Fatal("MCP roundtrip failed", err)
		}
		if _, _, err := repo.Get(ctx, "another-workspace", id); !errors.Is(err, modelerrors.ErrNotFound) {
			t.Fatal("MCP read crossed workspace boundary", err)
		}
		other := server
		other.WorkspaceID = "another-workspace"
		if _, err := repo.Update(ctx, other, "changed"); !errors.Is(err, modelerrors.ErrNotFound) {
			t.Fatal("MCP update crossed workspace boundary", err)
		}
		if err := repo.Delete(ctx, "another-workspace", id); !errors.Is(err, modelerrors.ErrNotFound) {
			t.Fatal("MCP delete crossed workspace boundary", err)
		}
		server.Args = []string{}
		updated, err := repo.Update(ctx, server, "")
		if err != nil || updated.HasHeaders || updated.HasEnvironment || len(updated.Args) != 0 {
			t.Fatal("MCP clear failed", err)
		}
		if transport == "stdio" {
			_, err = database.pool.Exec(ctx, `UPDATE mcp_servers SET encrypted_headers='invalid' WHERE id=$1`, id)
			if err == nil {
				t.Fatal("database accepted mixed transport credentials")
			}
		}
		if err := repo.Delete(ctx, workspaceID, id); err != nil {
			t.Fatal(err)
		}
	}
	// Legacy inserts continue to default to HTTP after the additive migration.
	id, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.pool.Exec(ctx, `INSERT INTO mcp_servers(id,workspace_id,name,url) VALUES($1,$2,'legacy MCP','https://example.test')`, id, workspaceID); err != nil {
		t.Fatal(err)
	}
	server, _, err := repo.Get(ctx, workspaceID, id)
	if err != nil || server.Transport != "http" {
		t.Fatal("legacy HTTP insert changed", err)
	}
	if err := repo.Delete(ctx, workspaceID, id); err != nil {
		t.Fatal(err)
	}
}
