package postgres

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/contracts/mcptransport"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

func TestSessionSnapshotStripsMCPCredentialsAndRetainsStdioSettings(t *testing.T) {
	config := orchestrationmodel.RunConfig{RuntimeConfig: orchestrationmodel.RuntimeConfig{MCPServers: []orchestrationmodel.MCPServer{
		{ID: "http", URL: "https://example.test", Headers: map[string]string{"Authorization": "http-private"}, Transport: mcptransport.HTTP},
		{ID: "stdio", Transport: mcptransport.Stdio, Command: "/server", Args: []string{"--literal"}, Environment: map[string]string{"TOKEN": "stdio-private"}},
	}}}
	data, err := (&AgentLaunchPostgresRepository{}).sessionSnapshot(config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private") {
		t.Fatal("MCP credential persisted in plaintext")
	}
	var restored orchestrationmodel.RunConfig
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	local := restored.MCPServers[1]
	if _, exists := local.Environment["TOKEN"]; !exists || local.Environment["TOKEN"] != "" || local.Command != "/server" || local.Args[0] != "--literal" {
		t.Fatal("snapshot lost pinned stdio configuration")
	}
	if config.MCPServers[1].Environment["TOKEN"] != "stdio-private" {
		t.Fatal("snapshot mutated live credentials")
	}
}
