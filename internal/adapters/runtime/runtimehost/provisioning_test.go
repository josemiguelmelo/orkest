package runtimehost

import (
	"os"
	"path/filepath"
	"testing"

	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

func TestProvisioningRejectsEscapesAndReservedEnvironment(t *testing.T) {
	root := t.TempDir()
	if _, err := applyProvisioningPlan(root, nil, []string{"HOME"}, []string{"OPENCODE_"}, agentruntime.ProvisioningPlan{Files: map[string][]byte{"../escape": []byte("bad")}}); err == nil {
		t.Fatal("path escape was accepted")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape")); !os.IsNotExist(err) {
		t.Fatalf("escaped file exists or could not be checked: %v", err)
	}
	for _, entry := range []string{"HOME=/tmp/home", "OPENCODE_CONFIG_CONTENT=secret"} {
		if _, err := applyProvisioningPlan(root, []string{entry}, []string{"HOME"}, []string{"OPENCODE_"}, agentruntime.ProvisioningPlan{}); err == nil {
			t.Fatalf("reserved variable accepted: %s", entry)
		}
	}
}

func TestProvisioningRejectsSymlinkEscape(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "external")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	_, err := applyProvisioningPlan(root, nil, nil, nil, agentruntime.ProvisioningPlan{Files: map[string][]byte{"external/escaped": []byte("secret")}})
	if err == nil {
		t.Fatal("symlink escape was accepted")
	}
	if _, err := os.Stat(filepath.Join(outside, "escaped")); !os.IsNotExist(err) {
		t.Fatalf("provisioner wrote outside runtime root: %v", err)
	}
}

func TestProvisioningRejectsImmutableFileConflict(t *testing.T) {
	root := t.TempDir()
	plan := agentruntime.ProvisioningPlan{Files: map[string][]byte{"config/agent.md": []byte("first")}}
	if _, err := applyProvisioningPlan(root, nil, nil, nil, plan); err != nil {
		t.Fatal(err)
	}
	plan.Files["config/agent.md"] = []byte("changed")
	if _, err := applyProvisioningPlan(root, nil, nil, nil, plan); err == nil {
		t.Fatal("incompatible durable file was overwritten")
	}
}
