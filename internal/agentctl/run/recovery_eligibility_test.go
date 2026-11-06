package run

import (
	"context"
	"errors"
	"testing"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
)

type countingRuntimeHost struct{ launchCalls, restoreCalls int }

func (h *countingRuntimeHost) Runtime() coreruntime.Runtime { return coreruntime.OpenCode }
func (h *countingRuntimeHost) Version() string              { return "fixture-v1" }
func (h *countingRuntimeHost) Launch(context.Context, runconfig.RuntimeConfig) (agentruntime.RuntimeSession, error) {
	h.launchCalls++
	return nil, errors.New("unexpected launch")
}

func (h *countingRuntimeHost) Restore(context.Context, runconfig.RuntimeConfig) (agentruntime.RuntimeSession, error) {
	h.restoreCalls++
	return nil, errors.New("unexpected restore")
}

func TestRestorationEligibilityExcludesCancelledAndFinishedRuns(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		events []interaction.EventType
		want   bool
	}{
		{name: "runtime stopped", events: []interaction.EventType{interaction.EventRuntimeStopped}, want: true},
		{name: "prompt failed", events: []interaction.EventType{interaction.EventPromptFailed}},
		{name: "cancelled", events: []interaction.EventType{interaction.EventCancelRequested}},
		{name: "run completed", events: []interaction.EventType{interaction.EventRunCompleted}},
		{name: "late activity", events: []interaction.EventType{interaction.EventRuntimeStopped, interaction.EventUpdate, interaction.EventPermissionRequested, interaction.EventPermissionResponse}, want: true},
		{name: "unknown event", events: []interaction.EventType{interaction.EventRuntimeStopped, interaction.EventType("future_event")}, want: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			process := &fakeRuntimeSession{done: make(chan struct{})}
			host := &fakeRuntimeHost{process: process}
			controller := NewRunController(context.Background(), host, t.TempDir(), nil)
			defer func() { _ = controller.Close() }()
			config := runConfig()
			config.Runtime = coreruntime.OpenCode
			config.TaskSessionID = "conversation"
			if err := controller.Configure(config); err != nil {
				t.Fatal(err)
			}
			if err := controller.Launch(); err != nil {
				t.Fatal(err)
			}
			conversation := controller.currentConversation()
			conversation.mu.Lock()
			for _, event := range testCase.events {
				conversation.state.apply(interaction.Event{Type: event})
			}
			conversation.mu.Unlock()
			if err := process.Close(); err != nil {
				t.Fatal(err)
			}
			if got := controller.CanAutoRestore(); got != testCase.want {
				t.Fatalf("%s restoration eligibility = %v, want %v", testCase.name, got, testCase.want)
			}
		})
	}
}

func TestRecoverDoesNotRestoreFinishedRun(t *testing.T) {
	dir := t.TempDir()
	config := recoveryConfig()
	if err := (checkpointStore{StateDir: dir}).Save(config); err != nil {
		t.Fatal(err)
	}
	recordRecoveryHistory(t, dir, interaction.Event{Type: interaction.EventRunCompleted})
	host := &countingRuntimeHost{}
	controller := NewRunController(context.Background(), host, dir, nil)
	defer func() { _ = controller.Close() }()
	if err := controller.RecoverFromCheckpoint(""); err != nil {
		t.Fatal(err)
	}
	if host.launchCalls != 0 || host.restoreCalls != 0 {
		t.Fatalf("finished run contacted host: launch=%d restore=%d", host.launchCalls, host.restoreCalls)
	}
	status := controller.Status()
	if !status.RunEnded || status.State != StateExited {
		t.Fatalf("finished run status: %+v", status)
	}
	if err := controller.RestoreSession(); !errors.Is(err, ErrLaunchConflict) {
		t.Fatalf("finished run restoration = %v", err)
	}
}
