package claudecode

import (
	"reflect"
	"testing"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
)

func TestDescriptorPinsACPAdapterAndReservesConfiguration(t *testing.T) {
	descriptor := (Profile{}).Descriptor()
	if descriptor.Runtime != coreruntime.ClaudeCode || descriptor.Version != Version || descriptor.Executable != "claude-agent-acp" {
		t.Fatalf("unexpected runtime descriptor: %+v", descriptor)
	}
	if descriptor.ReplayCoverageVerified || len(descriptor.VersionArguments) != 1 || descriptor.VersionArguments[0] != "--version" {
		t.Fatalf("unexpected qualification or version settings: %+v", descriptor)
	}
}

func TestProtocolOptionsCarryModelInstructionsSkillsAndMCP(t *testing.T) {
	settings := agentruntime.ResolvedRuntimeSettings{
		Agent:      configmodel.AgentDefinition{Instructions: "Follow the task."},
		Model:      configmodel.ModelSelection{Provider: "anthropic", ID: "claude-sonnet-test"},
		Skills:     []configmodel.Skill{{ID: "review", SkillMD: "Review carefully."}},
		MCPServers: []configmodel.MCPServer{{ID: "mcp-1", Name: "Tracker", URL: "https://mcp.example.test", Headers: map[string]string{"Authorization": "secret"}, Transport: "http"}},
	}
	got := (Profile{}).ProtocolSessionOptions(settings, agentruntime.ProvisioningPaths{})
	if got.Metadata["systemPrompt"] != "Follow the task.\n\n## Skill: review\n\nReview carefully." {
		t.Fatalf("unexpected system prompt: %#v", got.Metadata["systemPrompt"])
	}
	claude := got.Metadata["claudeCode"].(map[string]any)
	options := claude["options"].(map[string]any)
	if options["model"] != "claude-sonnet-test" || options["allowDangerouslySkipPermissions"] != false {
		t.Fatalf("unexpected Claude Code options: %#v", options)
	}
	if !reflect.DeepEqual(got.MCPServers, settings.MCPServers) {
		t.Fatalf("MCP servers were not forwarded: %#v", got.MCPServers)
	}
}

func TestAutonomousPolicySelectsClaudeBypassMode(t *testing.T) {
	settings := agentruntime.ResolvedRuntimeSettings{Agent: configmodel.AgentDefinition{PermissionPolicy: configmodel.PermissionPolicyAutonomous}}
	options := (Profile{}).ProtocolSessionOptions(settings, agentruntime.ProvisioningPaths{})
	claude := options.Metadata["claudeCode"].(map[string]any)
	claudeOptions := claude["options"].(map[string]any)
	if claudeOptions["allowDangerouslySkipPermissions"] != true {
		t.Fatalf("Claude bypass permission was not enabled: %#v", claudeOptions)
	}
	if got := (Profile{}).NativeSessionMode(settings); got != "bypassPermissions" {
		t.Fatalf("native mode = %q, want bypassPermissions", got)
	}
}

func TestProvisioningIsScopedAndUsesACPv1(t *testing.T) {
	paths := agentruntime.ProvisioningPaths{RuntimeRoot: "/state/run", WorkDir: "/workspace", ConfigSubdir: "config-abc"}
	plan, err := (Profile{}).BuildProvisioningPlan(agentruntime.ResolvedRuntimeSettings{}, paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 0 {
		t.Fatalf("unexpected rendered files: %#v", plan.Files)
	}
	wantEnv := []string{"PATH=" + agentruntime.DefaultRuntimePath, "HOME=/state/run/home", "CLAUDE_CONFIG_DIR=/state/run/config-abc/claude", "CLAUDE_AGENT_ACP_PROTOCOL=v1"}
	if !reflect.DeepEqual(plan.Environment, wantEnv) {
		t.Fatalf("unexpected environment: %#v", plan.Environment)
	}
	if _, err := (Profile{}).BuildProvisioningPlan(agentruntime.ResolvedRuntimeSettings{}, agentruntime.ProvisioningPaths{RuntimeRoot: "/state", WorkDir: "/work", ConfigSubdir: "../escape"}); err == nil {
		t.Fatal("accepted a configuration path outside runtime state")
	}
}
