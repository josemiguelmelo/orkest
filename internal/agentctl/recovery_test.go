package agentctl

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	coreruntime "github.com/josemiguelmelo/orkest/internal/core/runtime"
)

type recoveryProvider struct{ process *recoveryProcess }

func (p recoveryProvider) Runtime() coreruntime.Runtime { return "opencode" }

func (p recoveryProvider) Launch(context.Context, RunConfig) (Process, error) { return p.process, nil }

func (p recoveryProvider) Restore(context.Context, RunConfig) (Process, error) { return p.process, nil }

type recoveryProcess struct {
	testProcess
	calls   chan string
	release chan struct{}
	safe    bool
	fail    bool
}

func (p *recoveryProcess) Prompt(ctx context.Context, text string, _ Observer) (string, error) {
	p.calls <- text
	select {
	case <-p.release:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if p.fail {
		return "", errors.New("fixture failure")
	}
	return "end_turn", nil
}

func (p *recoveryProcess) Cancel(context.Context) error    { return nil }
func (p *recoveryProcess) CanContinue(string, string) bool { return p.safe }

func newRecoveryProcess() *recoveryProcess {
	return &recoveryProcess{testProcess: testProcess{done: make(chan struct{})}, calls: make(chan string, 10), release: make(chan struct{}, 10)}
}

func recoveryConfig() RunConfig {
	config := runConfig()
	config.TaskSessionID = "task-session"
	return config
}

func recordRecoveryHistory(t *testing.T, dir string, events ...Event) {
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

func waitRecoveryEvent(t *testing.T, events <-chan Event, kind EventType) Event {
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
	process := newRecoveryProcess()
	controller := NewRunControllerWithRuntimeProvider(context.Background(), recoveryProvider{process}, t.TempDir())
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
	waitRecoveryEvent(t, events, EventRunCompleted)
	if err := controller.Prompt("first", "first"); err != nil {
		t.Fatal("deduplication lost at completion", err)
	}
	if err := controller.Prompt("fourth", "fourth"); !errors.Is(err, ErrConversationInterrupted) {
		t.Fatal("finished run accepted prompt", err)
	}
}

func TestRestoreQueuedAndUncertainCommands(t *testing.T) {
	for _, started := range []bool{false, true} {
		t.Run(map[bool]string{false: "queued", true: "uncertain"}[started], func(t *testing.T) {
			dir := t.TempDir()
			history := []Event{promptAcceptedEvent("one", "original")}
			if started {
				history = append(history, Event{Type: EventPromptStarted, CommandID: "one"})
			}
			history = append(history, promptAcceptedEvent("two", "next"))
			recordRecoveryHistory(t, dir, history...)
			process := newRecoveryProcess()
			controller := NewRunControllerWithRuntimeProvider(context.Background(), recoveryProvider{process}, dir)
			defer func() { _ = controller.Close() }()
			if err := controller.Configure(recoveryConfig()); err != nil {
				t.Fatal(err)
			}
			if err := controller.Restore(); err != nil {
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
				if err := controller.RespondPermission(PermissionResponse{TaskID: "task", RunID: "run", SessionID: "session", RequestID: "permission-1", OptionID: "allow"}); !errors.Is(err, ErrPermission) {
					t.Fatal("stale approval accepted")
				}
				if err := controller.Queue(false); err != nil {
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
	recordRecoveryHistory(t, dir, promptAcceptedEvent("one", "original"), Event{Type: EventPromptStarted, CommandID: "one"})
	process := newRecoveryProcess()
	process.safe = true
	controller := NewRunControllerWithRuntimeProvider(context.Background(), recoveryProvider{process}, dir)
	defer func() { _ = controller.Close() }()
	if err := controller.Configure(recoveryConfig()); err != nil {
		t.Fatal(err)
	}
	if err := controller.Restore(); err != nil {
		t.Fatal(err)
	}
	if actual := <-process.calls; actual == "original" || actual == "" {
		t.Fatal("expected continuation instruction", actual)
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
	process := newRecoveryProcess()
	process.fail = true
	controller := NewRunControllerWithRuntimeProvider(context.Background(), recoveryProvider{process}, t.TempDir())
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
	waitRecoveryEvent(t, events, EventQueuePaused)
	if err := controller.Queue(true); err != nil {
		t.Fatal(err)
	}
	waitRecoveryEvent(t, events, EventRunFailed)
	status, err := controller.Command("two")
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
		event := Event{Sequence: sequence, OrganizationID: config.OrganizationID, WorkspaceID: config.WorkspaceID, TaskID: config.TaskID, RunID: config.RunID, Type: EventUpdate}
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
