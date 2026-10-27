package run

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
)

// blockedFile lets a test hold a commit in flight without relying on slow disks.
type blockedFile struct {
	successfulFile
	started     chan struct{}
	startedOnce sync.Once
	release     chan struct{}
}

func (f *blockedFile) Sync() error {
	f.startedOnce.Do(func() { close(f.started) })
	<-f.release
	return nil
}

func TestStorageCommitAllowsStatusAndShutdownAdmission(t *testing.T) {
	process := &chatRuntimeSession{fakeRuntimeSession: fakeRuntimeSession{done: make(chan struct{})}, calls: make(chan struct{}, 1), release: make(chan struct{})}
	controller := NewRunController(context.Background(), chatRuntimeHost{process}, t.TempDir(), nil)
	defer func() { _ = controller.Close() }()
	if err := controller.Configure(runConfig()); err != nil {
		t.Fatal(err)
	}
	if err := controller.Launch(); err != nil {
		t.Fatal(err)
	}
	c := controller.conversation
	file := &blockedFile{started: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	defer release.Do(func() { close(file.release) })
	c.journal = &Journal{file: file}
	c.state.commands = make(map[string]commandRecord)
	c.pending = make(map[string]*pendingPermission)
	_, events, unsubscribe, err := controller.Subscribe(0)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	prompted := make(chan error, 1)
	go func() { prompted <- controller.Prompt("first", "hello") }()
	select {
	case <-file.started:
	case <-time.After(time.Second):
		t.Fatal("commit did not start")
	}
	checked := make(chan struct{})
	go func() { _ = controller.Status(); c.stop(); close(checked) }()
	select {
	case <-checked:
	case <-time.After(time.Second):
		t.Fatal("storage blocked status or shutdown admission")
	}
	select {
	case event := <-events:
		t.Fatalf("published before sync: %+v", event)
	default:
	}
	release.Do(func() { close(file.release) })
	select {
	case err := <-prompted:
		if !errors.Is(err, interaction.ErrConversation) {
			t.Fatalf("shutdown admitted prompt: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("prompt did not finish")
	}
	select {
	case <-process.calls:
		t.Fatal("provider invoked after shutdown")
	default:
	}
}

func TestFailedPermissionPersistenceWakesWaiters(t *testing.T) {
	for _, syncFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "write", true: "sync"}[syncFailure], func(t *testing.T) {
			c := newConversation(context.Background(), t.TempDir(), nil)
			process := &fakeRuntimeSession{done: make(chan struct{})}
			c.bind(runConfig(), process)
			defer func() { _ = c.close(); _ = process.Close() }()
			c.journal = &Journal{file: successfulFile{}}
			c.pending = make(map[string]*pendingPermission)
			c.state.activeCommandID = "first"
			_, events, unsubscribe, err := c.Subscribe(0)
			if err != nil {
				t.Fatal(err)
			}
			defer unsubscribe()
			results := make(chan error, 2)
			for range 2 {
				go func() {
					option, err := c.waitPermission(context.Background(), "first", interaction.Permission{Options: []interaction.PermissionOption{{ID: "allow"}}})
					if option != "" {
						results <- errors.New("permission was approved")
						return
					}
					results <- err
				}()
			}
			var request interaction.Event
			for range 2 {
				select {
				case request = <-events:
				case <-time.After(time.Second):
					t.Fatal("permission not requested")
				}
			}
			c.eventMu.Lock()
			c.journal.file = failingFile{syncFailure: syncFailure}
			c.eventMu.Unlock()
			response := interaction.PermissionResponse{TaskID: "task", RunID: "run", SessionID: "session", RequestID: request.RequestID, OptionID: "allow"}
			if err := c.RespondPermission(response); !errors.Is(err, ErrJournal) {
				t.Fatalf("response: %v", err)
			}
			for range 2 {
				select {
				case err := <-results:
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("permission result: %v", err)
					}
				case <-time.After(time.Second):
					t.Fatal("permission remained blocked after journal failure")
				}
			}
			if c.Snapshot().State != StateInterrupted {
				t.Fatal("interruption missing")
			}
			if err := c.RespondPermission(response); !errors.Is(err, interaction.ErrPermission) {
				t.Fatalf("accepted stale response: %v", err)
			}
			select {
			case event := <-events:
				t.Fatalf("published failed response: %+v", event)
			default:
			}
		})
	}
}
