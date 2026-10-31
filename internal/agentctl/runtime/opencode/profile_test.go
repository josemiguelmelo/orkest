package opencode

import (
	"encoding/json"
	"strings"
	"testing"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

func TestPrepareEnvironmentIncludesConfiguredHTTPMCPServers(t *testing.T) {
	root := t.TempDir()
	settings := agentruntime.ResolvedRuntimeSettings{
		MCPServers: []runconfig.MCPServer{{ID: "server-id", Name: "Example MCP", URL: "https://example.test/mcp", Headers: map[string]string{"Authorization": "Bearer private"}}},
	}
	plan, err := (Profile{}).BuildProvisioningPlan(settings, agentruntime.ProvisioningPaths{RuntimeRoot: root, WorkDir: t.TempDir(), ConfigSubdir: "config-run"})
	if err != nil {
		t.Fatal(err)
	}
	var encoded string
	for _, entry := range plan.Environment {
		if strings.HasPrefix(entry, "OPENCODE_CONFIG_CONTENT=") {
			encoded = strings.TrimPrefix(entry, "OPENCODE_CONFIG_CONTENT=")
		}
	}
	var runtimeConfig struct {
		MCP struct {
			Servers map[string]struct {
				Type    string            `json:"type"`
				URL     string            `json:"url"`
				OAuth   *bool             `json:"oauth"`
				Headers map[string]string `json:"headers"`
			} `json:"servers"`
		} `json:"mcp"`
	}
	if err = json.Unmarshal([]byte(encoded), &runtimeConfig); err != nil {
		t.Fatal(err)
	}
	server, ok := runtimeConfig.MCP.Servers["example-mcp-server-id"]
	if !ok || server.Type != "remote" || server.URL != "https://example.test/mcp" || server.OAuth == nil || *server.OAuth || server.Headers["Authorization"] != "Bearer private" {
		t.Fatalf("unexpected runtime MCP configuration: %#v", runtimeConfig.MCP.Servers)
	}
}

func TestAutonomousPolicyAllowsOpenCodeActions(t *testing.T) {
	plan, err := (Profile{}).BuildProvisioningPlan(agentruntime.ResolvedRuntimeSettings{Agent: runconfig.AgentDefinition{PermissionPolicy: runconfig.PermissionPolicyAutonomous}}, agentruntime.ProvisioningPaths{RuntimeRoot: t.TempDir(), WorkDir: t.TempDir(), ConfigSubdir: "config-run"})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range plan.Environment {
		if strings.HasPrefix(entry, "OPENCODE_CONFIG_CONTENT=") {
			var config struct {
				Permissions []permissionRule `json:"permissions"`
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(entry, "OPENCODE_CONFIG_CONTENT=")), &config); err != nil {
				t.Fatal(err)
			}
			if len(config.Permissions) != 1 || config.Permissions[0] != (permissionRule{Action: "*", Resource: "*", Effect: "allow"}) {
				t.Fatalf("unexpected autonomous policy: %#v", config.Permissions)
			}
			return
		}
	}
	t.Fatal("OpenCode runtime configuration was missing")
}
