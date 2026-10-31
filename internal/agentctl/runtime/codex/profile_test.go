package codex

import (
	"encoding/json"
	"strings"
	"testing"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	coreruntime "github.com/josemiguelmelo/orkest/internal/core/runtime"
)

func TestDescriptorPinsACPAdapterAndIsolatesCodexState(t *testing.T) {
	descriptor := (Profile{}).Descriptor()
	if descriptor.Runtime != coreruntime.Codex || descriptor.Version != "2.3.0" || descriptor.VersionOutputPrefix != "@agentclientprotocol/codex-acp " || descriptor.Executable != "codex-acp" {
		t.Fatalf("unexpected Codex descriptor: %+v", descriptor)
	}
	if descriptor.ReplayCoverageVerified {
		t.Fatal("Codex replay coverage must remain unverified until live qualification")
	}
}

func TestProvisioningUsesModelAndWorkspaceInstructions(t *testing.T) {
	settings := agentruntime.ResolvedRuntimeSettings{
		Agent:  runconfig.AgentDefinition{Name: "Work", Instructions: "Do the task", PermissionPolicy: runconfig.PermissionPolicyAutonomous},
		Model:  runconfig.ModelSelection{Provider: "openai", ID: "gpt-codex"},
		Skills: []runconfig.Skill{{ID: "review", SkillMD: "Review carefully"}},
	}
	plan, err := (Profile{}).BuildProvisioningPlan(settings, agentruntime.ProvisioningPaths{RuntimeRoot: "/state", WorkDir: "/work", ConfigSubdir: "run-config"})
	if err != nil {
		t.Fatal(err)
	}
	var codexConfig map[string]string
	for _, value := range plan.Environment {
		if strings.HasPrefix(value, "CODEX_CONFIG=") {
			if err = json.Unmarshal([]byte(strings.TrimPrefix(value, "CODEX_CONFIG=")), &codexConfig); err != nil {
				t.Fatalf("decode Codex configuration: %v", err)
			}
		}
	}
	if codexConfig["model"] != "gpt-codex" || !strings.Contains(codexConfig["developer_instructions"], "Review carefully") {
		t.Fatalf("Codex configuration omitted model or instructions: %#v", codexConfig)
	}
	if got := (Profile{}).NativeSessionMode(settings); got != "agent-full-access" {
		t.Fatalf("autonomous Codex mode = %q, want agent-full-access", got)
	}
	if !strings.Contains(strings.Join(plan.Environment, "\n"), "APP_SERVER_LOGS=/state/state/codex/logs") {
		t.Fatalf("Codex logs are not scoped to durable runtime state: %#v", plan.Environment)
	}
	environment := strings.Join(plan.Environment, "\n")
	if !strings.Contains(environment, "NO_BROWSER=1") || !strings.Contains(environment, "INITIAL_AGENT_MODE=agent-full-access") {
		t.Fatalf("Codex headless permission defaults are missing: %#v", plan.Environment)
	}
	if !strings.Contains(environment, `DEFAULT_AUTH_REQUEST={"methodId":"api-key"}`) {
		t.Fatalf("Codex API-key authentication request is missing: %#v", plan.Environment)
	}
}

func TestAskPolicyStartsCodexInWorkspaceWriteMode(t *testing.T) {
	plan, err := (Profile{}).BuildProvisioningPlan(agentruntime.ResolvedRuntimeSettings{}, agentruntime.ProvisioningPaths{RuntimeRoot: "/state", WorkDir: "/work", ConfigSubdir: "run-config"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(plan.Environment, "\n"), "INITIAL_AGENT_MODE=workspace-write") {
		t.Fatalf("ask policy did not retain workspace-write mode: %#v", plan.Environment)
	}
}

func TestProvisioningRejectsInvalidSkill(t *testing.T) {
	settings := agentruntime.ResolvedRuntimeSettings{Skills: []runconfig.Skill{{ID: "../escape", SkillMD: "bad"}}}
	paths := agentruntime.ProvisioningPaths{RuntimeRoot: "/state", WorkDir: "/work", ConfigSubdir: "run-config"}
	if _, err := (Profile{}).BuildProvisioningPlan(settings, paths); err == nil {
		t.Fatal("invalid skill ID was accepted")
	}
}
