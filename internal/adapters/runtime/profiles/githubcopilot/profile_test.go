package githubcopilot

import (
	"strings"
	"testing"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	"github.com/josemiguelmelo/orkest/internal/contracts/mcptransport"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
)

func TestDescriptorUsesPinnedCopilotACPAndKeepsReplayUnqualified(t *testing.T) {
	descriptor := (Profile{}).Descriptor()
	if descriptor.Runtime != coreruntime.GitHubCopilot || descriptor.Version != "1.0.105" || descriptor.Executable != "copilot" || strings.Join(descriptor.Arguments, " ") != "--acp --stdio" {
		t.Fatalf("unexpected Copilot descriptor: %+v", descriptor)
	}
	if descriptor.ReplayCoverageVerified {
		t.Fatal("Copilot replay coverage must remain unverified until live qualification")
	}
}

func TestProvisioningCarriesInstructionsSkillsModelAndMCP(t *testing.T) {
	settings := agentruntime.ResolvedRuntimeSettings{
		Agent:      configmodel.AgentDefinition{Instructions: "Do the task"},
		Model:      configmodel.ModelSelection{Provider: "github-copilot", ID: "gpt-4.1"},
		Skills:     []configmodel.Skill{{ID: "review", SkillMD: "Review carefully"}},
		MCPServers: []configmodel.MCPServer{{ID: "tools", Name: "Tools", URL: "https://mcp.example.test", Headers: map[string]string{"Authorization": "secret"}, Transport: mcptransport.HTTP}},
	}
	paths := agentruntime.ProvisioningPaths{RuntimeRoot: "/state", WorkDir: "/work", ConfigSubdir: "run-config"}
	plan, err := (Profile{}).BuildProvisioningPlan(settings, paths)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(plan.Files["run-config/copilot/copilot-instructions.md"]); !strings.Contains(got, "Do the task") {
		t.Fatalf("Copilot instructions were not provisioned: %q", got)
	}
	if got := string(plan.Files["run-config/copilot/skills/review/SKILL.md"]); !strings.Contains(got, "Review carefully") {
		t.Fatalf("Copilot skill was not provisioned: %q", got)
	}
	environment := strings.Join(plan.Environment, "\n")
	for _, value := range []string{"COPILOT_HOME=/state/run-config/copilot", "COPILOT_MODEL=gpt-4.1", "COPILOT_AUTO_UPDATE=false"} {
		if !strings.Contains(environment, value) {
			t.Fatalf("Copilot environment omitted %q: %#v", value, plan.Environment)
		}
	}
	options := (Profile{}).ProtocolSessionOptions(settings, paths)
	if len(options.MCPServers) != 1 || options.MCPServers[0].Headers["Authorization"] != "secret" {
		t.Fatalf("Copilot ACP session options omitted MCP configuration: %#v", options)
	}
}

func TestAutonomousPermissionPolicyEnablesCopilotTools(t *testing.T) {
	settings := agentruntime.ResolvedRuntimeSettings{Agent: configmodel.AgentDefinition{PermissionPolicy: configmodel.PermissionPolicyAutonomous}, Model: configmodel.ModelSelection{ID: "gpt-4.1"}}
	plan, err := (Profile{}).BuildProvisioningPlan(settings, agentruntime.ProvisioningPaths{RuntimeRoot: "/state", WorkDir: "/work", ConfigSubdir: "run-config"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(plan.Environment, "\n"), "COPILOT_ALLOW_ALL=true") {
		t.Fatalf("autonomous permission policy was not applied: %#v", plan.Environment)
	}
}
