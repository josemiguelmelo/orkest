package run

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	runmodel "github.com/josemiguelmelo/orkest/internal/agentctl/run/model"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
)

type recoveryRuntimeHost struct{ process *recoveryRuntimeSession }

func (p recoveryRuntimeHost) Runtime() coreruntime.Runtime { return "opencode" }

func (p recoveryRuntimeHost) Launch(context.Context, configmodel.RuntimeConfig) (agentruntime.RuntimeSession, error) {
	return p.process, nil
}

func (p recoveryRuntimeHost) Restore(context.Context, configmodel.RuntimeConfig) (agentruntime.RuntimeSession, error) {
	return p.process, nil
}

type recoveryRuntimeSession struct {
	fakeRuntimeSession
	calls   chan string
	release chan struct{}
	safe    bool
	fail    bool
}

func (p *recoveryRuntimeSession) Prompt(ctx context.Context, command interaction.Command, _ agentruntime.PromptObserver) (agentruntime.StopReason, error) {
	p.calls <- command.Text
	select {
	case <-p.release:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if p.fail {
		return "", errors.New("fixture failure")
	}
	return agentruntime.StopReasonEndTurn, nil
}

func (p *recoveryRuntimeSession) Cancel(context.Context) error { return nil }

func (p *recoveryRuntimeSession) Reconcile(context.Context, interaction.Command) (agentruntime.RecoveryDecision, error) {
	if p.safe {
		return agentruntime.RecoveryDecision{Action: agentruntime.RecoveryContinue}, nil
	}
	return agentruntime.RecoveryDecision{Action: agentruntime.RecoveryPause}, nil
}

func newRecoverySession() *recoveryRuntimeSession {
	return &recoveryRuntimeSession{fakeRuntimeSession: fakeRuntimeSession{done: make(chan struct{})}, calls: make(chan string, 10), release: make(chan struct{}, 10)}
}

func recoveryConfig() configmodel.RunConfig {
	config := runConfig()
	config.TaskSessionID = "task-session"
	return config
}

func recordRecoveryHistory(t *testing.T, dir string, events ...interaction.Event) {
	t.Helper()
	config := recoveryConfig()
	journal, err := openJournal(dir, config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = journal.Close() }()
	for _, event := range events {
		event.OrganizationID, event.WorkspaceID, event.TaskID, event.RunID, event.SessionID = config.OrganizationID, config.WorkspaceID, config.TaskID, config.RunID, "session"
		if _, err := journal.Append(event); err != nil {
			t.Fatal(err)
		}
	}
}

func waitRecoveryEvent(t *testing.T, events <-chan interaction.Event, kind interaction.EventType) interaction.Event {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if event.Type == kind {
				return event
			}
		case <-timer.C:
			t.Fatalf("missing %s", kind)
		}
	}
}

func TestQueueFIFOAndRunCompletion(t *testing.T) {
	process := newRecoverySession()
	controller := NewRunController(context.Background(), recoveryRuntimeHost{process}, t.TempDir(), nil, nil)
	defer func() { _ = controller.Close() }()
	if err := controller.Configure(recoveryConfig()); err != nil {
		t.Fatal(err)
	}
	if err := controller.Launch(); err != nil {
		t.Fatal(err)
	}
	_, events, stop, err := controller.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	for _, text := range []string{"first", "second", "third"} {
		if err := controller.Prompt(text, text); err != nil {
			t.Fatal(err)
		}
	}
	for _, text := range []string{"first", "second", "third"} {
		select {
		case actual := <-process.calls:
			if actual != text {
				t.Fatalf("got %s, want %s", actual, text)
			}
		case <-time.After(time.Second):
			t.Fatal("missing prompt")
		}
		process.release <- struct{}{}
	}
	waitRecoveryEvent(t, events, interaction.EventRunCompleted)
	if err := controller.Prompt("first", "first"); err != nil {
		t.Fatal("deduplication lost at completion", err)
	}
	if err := controller.Prompt("fourth", "fourth"); !errors.Is(err, interaction.ErrConversationInterrupted) {
		t.Fatal("finished run accepted prompt", err)
	}
}

func TestRestoreQueuedAndUncertainCommands(t *testing.T) {
	for _, started := range []bool{false, true} {
		t.Run(map[bool]string{false: "queued", true: "uncertain"}[started], func(t *testing.T) {
			dir := t.TempDir()
			history := []interaction.Event{promptAcceptedEvent("one", "original")}
			if started {
				history = append(history, interaction.Event{Type: interaction.EventPromptStarted, CommandID: "one"})
			}
			history = append(history, promptAcceptedEvent("two", "next"))
			recordRecoveryHistory(t, dir, history...)
			process := newRecoverySession()
			controller := NewRunController(context.Background(), recoveryRuntimeHost{process}, dir, nil, nil)
			defer func() { _ = controller.Close() }()
			if err := controller.Configure(recoveryConfig()); err != nil {
				t.Fatal(err)
			}
			if err := controller.RestoreSession(); err != nil {
				t.Fatal(err)
			}
			if started {
				if !controller.Status().QueuePaused {
					t.Fatal("uncertain work was not paused")
				}
				select {
				case <-process.calls:
					t.Fatal("uncertain prompt resent")
				default:
				}
				if err := controller.Prompt("one", "original"); err != nil {
					t.Fatal("accepted identity not reconciled", err)
				}
				if err := controller.RespondPermission(interaction.PermissionResponse{TaskID: "task", RunID: "run", SessionID: "session", RequestID: "permission-1", OptionID: "allow"}); !errors.Is(err, interaction.ErrPermission) {
					t.Fatal("stale approval accepted")
				}
				if err := controller.ResumeQueue(); err != nil {
					t.Fatal(err)
				}
				if actual := <-process.calls; actual != "next" {
					t.Fatal("original prompt repeated", actual)
				}
				process.release <- struct{}{}
			} else {
				if actual := <-process.calls; actual != "original" {
					t.Fatal(actual)
				}
				process.release <- struct{}{}
				if actual := <-process.calls; actual != "next" {
					t.Fatal(actual)
				}
				process.release <- struct{}{}
			}
		})
	}
}

func TestRestoreSafeContinuation(t *testing.T) {
	dir := t.TempDir()
	recordRecoveryHistory(t, dir, promptAcceptedEvent("one", "original"), interaction.Event{Type: interaction.EventPromptStarted, CommandID: "one"})
	process := newRecoverySession()
	process.safe = true
	controller := NewRunController(context.Background(), recoveryRuntimeHost{process}, dir, nil, nil)
	defer func() { _ = controller.Close() }()
	if err := controller.Configure(recoveryConfig()); err != nil {
		t.Fatal(err)
	}
	if err := controller.RestoreSession(); err != nil {
		t.Fatal(err)
	}
	if actual := <-process.calls; actual == "original" || actual == "" {
		t.Fatal("expected continuation instruction", actual)
	}
	if err := controller.Prompt("one", "original"); err != nil {
		t.Fatal("original command retry lost its identity after continuation", err)
	}
	if err := controller.Prompt("one", "different"); !errors.Is(err, interaction.ErrCommand) {
		t.Fatal("changed command was accepted after continuation", err)
	}
	status, err := controller.CommandStatus("one")
	if err != nil || status.Status != interaction.CommandRunning {
		t.Fatalf("continuation lost the command state: %+v %v", status, err)
	}
	process.release <- struct{}{}
}

func TestRestoreJournalTailAndCorruption(t *testing.T) {
	for _, tail := range []string{`{"partial":`, "corrupt\n"} {
		t.Run(tail, func(t *testing.T) {
			dir := t.TempDir()
			recordRecoveryHistory(t, dir, promptAcceptedEvent("one", "original"))
			files, err := filepath.Glob(filepath.Join(dir, "conversations", "*.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			file, err := os.OpenFile(files[0], os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := file.WriteString(tail); err != nil {
				t.Fatal(err)
			}
			_ = file.Close()
			journal, history, err := loadJournal(dir, recoveryConfig(), true)
			if tail == "corrupt\n" {
				if !errors.Is(err, ErrJournal) {
					t.Fatal("committed corruption accepted", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = journal.Close() }()
			if len(history) != 1 || journal.sequence != 1 {
				t.Fatal("committed history changed")
			}
		})
	}
}

func TestQueueFailurePausesAndDiscardDoesNotSucceed(t *testing.T) {
	process := newRecoverySession()
	process.fail = true
	controller := NewRunController(context.Background(), recoveryRuntimeHost{process}, t.TempDir(), nil, nil)
	defer func() { _ = controller.Close() }()
	if err := controller.Configure(recoveryConfig()); err != nil {
		t.Fatal(err)
	}
	if err := controller.Launch(); err != nil {
		t.Fatal(err)
	}
	_, events, stop, err := controller.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if err := controller.Prompt("one", "fail"); err != nil {
		t.Fatal(err)
	}
	if err := controller.Prompt("two", "queued"); err != nil {
		t.Fatal(err)
	}
	<-process.calls
	process.release <- struct{}{}
	waitRecoveryEvent(t, events, interaction.EventQueuePaused)
	if err := controller.DiscardQueue(); err != nil {
		t.Fatal(err)
	}
	waitRecoveryEvent(t, events, interaction.EventRunFailed)
	status, err := controller.CommandStatus("two")
	if err != nil || status.Status != "discarded" {
		t.Fatal(status, err)
	}
	select {
	case <-process.calls:
		t.Fatal("discarded command executed")
	default:
	}
}

func TestRestoreReplayBeyondMemoryWindow(t *testing.T) {
	dir := t.TempDir()
	config := recoveryConfig()
	journal, err := openJournal(dir, config)
	if err != nil {
		t.Fatal(err)
	}
	for sequence := uint64(1); sequence <= maxReplayEvents+2; sequence++ {
		event := interaction.Event{Sequence: sequence, OrganizationID: config.OrganizationID, WorkspaceID: config.WorkspaceID, TaskID: config.TaskID, RunID: config.RunID, Type: interaction.EventUpdate}
		data, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := journal.file.Write(append(data, '\n')); err != nil {
			t.Fatal(err)
		}
	}
	if err := journal.file.Sync(); err != nil {
		t.Fatal(err)
	}
	_ = journal.Close()
	restored, _, err := loadJournal(dir, config, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	events, err := restored.Replay(0)
	if err != nil || len(events) != maxReplayEvents+2 {
		t.Fatal("old committed history could not be replayed", len(events), err)
	}
	if len(restored.events) != maxReplayEvents {
		t.Fatal("memory replay window grew without bound")
	}
}

func (p recoveryRuntimeHost) Version() string { return "fixture-v1" }

type blockingReconciliation struct {
	*recoveryRuntimeSession
	entered chan struct{}
}

func (p *blockingReconciliation) Reconcile(ctx context.Context, _ interaction.Command) (agentruntime.RecoveryDecision, error) {
	close(p.entered)
	<-ctx.Done()
	return agentruntime.RecoveryDecision{}, ctx.Err()
}

func TestRestoreReconciliationAllowsStatusAndShutdown(t *testing.T) {
	dir := t.TempDir()
	recordRecoveryHistory(t, dir, promptAcceptedEvent("one", "original"), interaction.Event{Type: interaction.EventPromptStarted, CommandID: "one"})
	process := &blockingReconciliation{recoveryRuntimeSession: newRecoverySession(), entered: make(chan struct{})}
	controller := NewRunController(context.Background(), conversationRuntimeHost{process: process}, dir, nil, nil)
	defer func() { _ = controller.Close() }()
	if err := controller.Configure(recoveryConfig()); err != nil {
		t.Fatal(err)
	}
	restored := make(chan error, 1)
	go func() { restored <- controller.RestoreSession() }()
	select {
	case <-process.entered:
	case <-time.After(time.Second):
		t.Fatal("reconciliation did not start")
	}
	status := make(chan runmodel.RunStatus, 1)
	go func() { status <- controller.Status() }()
	select {
	case <-status:
	case <-time.After(time.Second):
		t.Fatal("reconciliation blocked status")
	}
	if err := controller.RestoreSession(); !errors.Is(err, ErrLaunchConflict) {
		t.Fatal("concurrent restoration was admitted", err)
	}
	stopped := make(chan error, 1)
	go func() { stopped <- controller.Close() }()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("reconciliation blocked shutdown")
	}
	select {
	case err := <-restored:
		if err == nil {
			t.Fatal("interrupted restoration succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("reconciliation ignored cancellation")
	}
}
