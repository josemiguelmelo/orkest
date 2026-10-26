package opencode

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

func TestMaterializeSkills(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "opencode", "skills")
	content := "---\nname: go-review\ndescription: Review Go code\n---\n\nReview the change."
	if err := materializeSkills(directory, []agentctl.Skill{{ID: "go-review", SkillMD: content}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "go-review", "SKILL.md")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content+"\n" {
		t.Fatalf("materialized skill = %q", got)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("skill file permissions = %v, err=%v", info, err)
	}
	if err := materializeSkills(directory, []agentctl.Skill{{ID: "../escape", SkillMD: content}}); err == nil {
		t.Fatal("invalid skill ID was accepted")
	}
}
