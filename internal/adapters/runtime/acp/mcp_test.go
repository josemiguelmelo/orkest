package acp

import (
	"encoding/json"
	"strings"
	"testing"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	"github.com/josemiguelmelo/orkest/internal/contracts/mcptransport"
)

func TestMCPTransportWireRepresentations(t *testing.T) {
	servers, err := mcpServers([]configmodel.MCPServer{
		{Name: "Remote", URL: "https://example.test", Headers: map[string]string{"Authorization": "private"}, Transport: mcptransport.HTTP},
		{Name: "Local", Transport: mcptransport.Stdio, Command: "/opt/mcp/server", Args: []string{"literal space", "$TOKEN"}, Environment: map[string]string{"TOKEN": "private"}},
		{Name: "Empty", Transport: mcptransport.Stdio, Command: "/empty"},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(servers)
	if err != nil {
		t.Fatal(err)
	}
	var values []map[string]json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		t.Fatal(err)
	}
	if string(values[0]["type"]) != `"http"` || string(values[2]["args"]) != "[]" || string(values[2]["env"]) != "[]" {
		t.Fatal("invalid ACP representation", string(data))
	}
	local := string(mustJSON(t, values[1]))
	if strings.Contains(local, `"url"`) || strings.Contains(local, `"headers"`) || !strings.Contains(local, `"command":"/opt/mcp/server"`) {
		t.Fatal("stdio configuration was encoded as HTTP", local)
	}
	if got := servers[1].Stdio; got == nil || got.Args[0] != "literal space" || got.Args[1] != "$TOKEN" || got.Env[0].Value != "private" {
		t.Fatal("stdio arguments or environment changed")
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestMCPRejectsMissingOrUnsupportedTransport(t *testing.T) {
	for _, transport := range []mcptransport.Transport{"", "sse"} {
		if _, err := mcpServers([]configmodel.MCPServer{{Name: "Tools", Transport: transport, URL: "https://example.test"}}); err == nil {
			t.Fatal("invalid transport accepted")
		}
	}
}
