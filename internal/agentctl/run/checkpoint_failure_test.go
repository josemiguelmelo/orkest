package run

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	runmodel "github.com/josemiguelmelo/orkest/internal/agentctl/run/model"
)

func TestUnreadableCheckpointBlocksReplacementAndCanRetrySavedState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "current-run.json")
	corrupt := []byte(`{"version":`)
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	session := newRecoverySession()
	controller := NewRunController(context.Background(), recoveryRuntimeHost{process: session}, dir, nil)
	defer func() { _ = controller.Close() }()
	if err := controller.RecoverFromCheckpoint("run"); !errors.Is(err, ErrJournal) {
		t.Fatal(err)
	}
	if err := controller.Configure(runConfig()); !errors.Is(err, ErrRecoveryBlocked) {
		t.Fatalf("configuration bypassed fence: %v", err)
	}
	status := controller.Status()
	if !status.RecoveryBlocked || status.State != runmodel.StateInterrupted || status.ExecutionAvailable || !controller.CheckpointRecoveryPending() {
		t.Fatalf("status=%+v", status)
	}
	if err := controller.Launch(); !errors.Is(err, ErrRecoveryBlocked) {
		t.Fatalf("launch=%v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(corrupt) {
		t.Fatal("saved checkpoint was modified", err)
	}
	// Retrying corrupt state must retain the same fence.
	if err = controller.RecoverFromCheckpoint("run"); !errors.Is(err, ErrJournal) {
		t.Fatal(err)
	}
	if !controller.Status().RecoveryBlocked {
		t.Fatal("retry removed fence")
	}
	// Repair the same checkpoint, then restore and successfully complete a follow-up.
	if err = (checkpointStore{StateDir: dir}).Save(recoveryConfig()); err != nil {
		t.Fatal(err)
	}
	if err = controller.RecoverFromCheckpoint("run"); err != nil {
		t.Fatal(err)
	}
	if controller.CheckpointRecoveryPending() || controller.Status().RecoveryBlocked {
		t.Fatal("saved-state repair did not clear checkpoint fence")
	}
	_, events, unsubscribe, err := controller.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	session.release <- struct{}{}
	if err = controller.Prompt("followup", "hello again"); err != nil {
		t.Fatal(err)
	}
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if event.Type == interaction.EventPromptCompleted && event.CommandID == "followup" {
				return
			}
		case <-timer.C:
			t.Fatal("restored checkpoint did not complete follow-up")
		}
	}
}
