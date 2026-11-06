package opencode

import (
	"testing"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

func TestProfileRendersSkillsWithoutFilesystemIO(t *testing.T) {
	content := "---\nname: go-review\ndescription: Review Go code\n---\n\nReview the change."
	plan, err := (Profile{}).BuildProvisioningPlan(agentruntime.ResolvedRuntimeSettings{Skills: []configmodel.Skill{{ID: "go-review", SkillMD: content}}}, agentruntime.ProvisioningPaths{RuntimeRoot: t.TempDir(), WorkDir: t.TempDir(), ConfigSubdir: "config-run"})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := plan.Files["config-run/opencode/skills/go-review/SKILL.md"]
	if !ok || string(got) != content+"\n" {
		t.Fatalf("profile skill output = %q, present=%v", got, ok)
	}
	if _, err := (Profile{}).BuildProvisioningPlan(agentruntime.ResolvedRuntimeSettings{Skills: []configmodel.Skill{{ID: "../escape", SkillMD: content}}}, agentruntime.ProvisioningPaths{RuntimeRoot: t.TempDir(), WorkDir: t.TempDir(), ConfigSubdir: "config-run"}); err == nil {
		t.Fatal("invalid skill ID was accepted")
	}
}
