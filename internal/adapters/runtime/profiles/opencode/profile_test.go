package opencode

import (
	"encoding/json"
	"strings"
	"testing"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

func TestPrepareEnvironmentIncludesConfiguredHTTPMCPServers(t *testing.T) {
	root := t.TempDir()
	settings := agentruntime.ResolvedRuntimeSettings{
		MCPServers: []configmodel.MCPServer{{ID: "server-id", Name: "Example MCP", URL: "https://example.test/mcp", Headers: map[string]string{"Authorization": "Bearer private"}}},
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
	plan, err := (Profile{}).BuildProvisioningPlan(agentruntime.ResolvedRuntimeSettings{Agent: configmodel.AgentDefinition{PermissionPolicy: configmodel.PermissionPolicyAutonomous}}, agentruntime.ProvisioningPaths{RuntimeRoot: t.TempDir(), WorkDir: t.TempDir(), ConfigSubdir: "config-run"})
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

func TestConfiguredModelIsExplicitForACPAndRuntimeDefaults(t *testing.T) {
	settings := agentruntime.ResolvedRuntimeSettings{Model: configmodel.ModelSelection{Provider: "openrouter", ID: "openai/gpt-5.4-mini"}}
	paths := agentruntime.ProvisioningPaths{RuntimeRoot: t.TempDir(), WorkDir: t.TempDir(), ConfigSubdir: "config-run"}
	plan, err := (Profile{}).BuildProvisioningPlan(settings, paths)
	if err != nil {
		t.Fatal(err)
	}
	want := "openrouter/openai/gpt-5.4-mini"
	for _, entry := range plan.Environment {
		if strings.HasPrefix(entry, "OPENCODE_CONFIG_CONTENT=") {
			var config struct {
				Model string `json:"model"`
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(entry, "OPENCODE_CONFIG_CONTENT=")), &config); err != nil {
				t.Fatal(err)
			}
			if config.Model != want {
				t.Fatalf("default model=%q, want %q", config.Model, want)
			}
			options := (Profile{}).ProtocolSessionOptions(settings, paths)
			if options.Model != want {
				t.Fatalf("ACP model=%q, want %q", options.Model, want)
			}
			return
		}
	}
	t.Fatal("runtime configuration missing")
}

func TestMixedMCPConfiguration(t *testing.T) {
	config, err := buildMCPConfiguration([]configmodel.MCPServer{
		{ID: "remote", Name: "Remote", URL: "https://example.test"},
		{ID: "local", Name: "Local", Transport: "stdio", Command: "/opt/mcp/server", Args: []string{"literal space", "$TOKEN"}, Environment: map[string]string{"TOKEN": "private"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	local := config.Servers["local-local"]
	if local.Type != "local" || local.URL != "" || local.OAuth != nil || local.Environment["TOKEN"] != "private" || strings.Join(local.Command, "|") != "/opt/mcp/server|literal space|$TOKEN" {
		t.Fatal("stdio configuration changed", local)
	}
	encoded, err := json.Marshal(local)
	if err != nil || strings.Contains(string(encoded), `"url"`) || strings.Contains(string(encoded), `"headers"`) || strings.Contains(string(encoded), `"oauth"`) {
		t.Fatal("HTTP fields leaked into local configuration", string(encoded), err)
	}
	if config.Servers["remote-remote"].Type != "remote" {
		t.Fatal("HTTP server lost")
	}
}
