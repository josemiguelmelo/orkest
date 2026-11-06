package run

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
)

func TestCheckpointStoragePreservesV1ShapeAndRunMatch(t *testing.T) {
	stateDir := t.TempDir()
	store := checkpointStore{StateDir: stateDir}
	config := configmodel.RunConfig{
		RuntimeConfig: configmodel.RuntimeConfig{
			OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", TaskSessionID: "conversation",
			RunID: "run", Runtime: coreruntime.OpenCode,
			Agent: configmodel.AgentDefinition{Name: "Work", Description: "Work", Instructions: "Complete the task."},
			Model: configmodel.ModelSelection{Provider: "provider", ID: "model"},
		},
	}
	if err := store.Save(config); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "current-run-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(stateDir, "current-run.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
		t.Fatalf("checkpoint changed v1 encoding:\n got %s\nwant %s", got, want)
	}
	if _, found, err := store.Load("another-run"); err != nil || found {
		t.Fatalf("mismatched run loaded: found=%v err=%v", found, err)
	}
	loaded, found, err := store.Load("run")
	if err != nil || !found || loaded.RunID != config.RunID || loaded.TaskSessionID != config.TaskSessionID {
		t.Fatalf("checkpoint did not restore: found=%v config=%+v err=%v", found, loaded, err)
	}
}
