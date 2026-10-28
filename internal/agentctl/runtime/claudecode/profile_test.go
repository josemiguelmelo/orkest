package claudecode

import (
	"testing"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

func TestBuildProvisioningPlan(t *testing.T) {
	plan, err := Profile{}.BuildProvisioningPlan(agentruntime.ResolvedRuntimeSettings{
		Agent: runconfig.AgentDefinition{Name: "Work Agent", Description: "Does work", Instructions: "Follow instructions"},
		Model: runconfig.ModelSelection{Provider: "anthropic", ID: "claude-sonnet-4-5"},
	}, agentruntime.ProvisioningPaths{RuntimeRoot: "/tmp/runtime", WorkDir: "/workspace/project", ConfigSubdir: "config-run"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Directories) == 0 {
		t.Fatal("expected directories")
	}
	if _, ok := plan.Files["config-run/claude/agents/work-agent.md"]; !ok {
		t.Fatal("expected agent file")
	}
	foundHome := false
	for _, env := range plan.Environment {
		if len(env) > 5 && env[:5] == "HOME=" {
			foundHome = true
		}
	}
	if !foundHome {
		t.Fatal("expected HOME environment")
	}
}
