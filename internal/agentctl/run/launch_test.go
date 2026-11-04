package run

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
)

type fakeRuntimeHost struct {
	calls   atomic.Int32
	err     error
	process *fakeRuntimeSession
}

func (p *fakeRuntimeHost) Runtime() coreruntime.Runtime { return coreruntime.OpenCode }
func (p *fakeRuntimeHost) Launch(context.Context, runconfig.RuntimeConfig) (agentruntime.RuntimeSession, error) {
	p.calls.Add(1)
	return p.process, p.err
}

type fakeRuntimeSession struct {
	done    chan struct{}
	once    sync.Once
	exitErr error
}

func (p *fakeRuntimeSession) Err() error {
	select {
	case <-p.done:
		return p.exitErr
	default:
		return nil
	}
}
func (p *fakeRuntimeSession) SessionID() string     { return "session" }
func (p *fakeRuntimeSession) Done() <-chan struct{} { return p.done }
func (p *fakeRuntimeSession) Close() error          { p.once.Do(func() { close(p.done) }); return nil }

func TestProviderLaunchOwnership(t *testing.T) {
	p := &fakeRuntimeHost{process: &fakeRuntimeSession{done: make(chan struct{})}}
	service := NewRunController(context.Background(), p, "", nil)
	defer func() { _ = service.Close() }()
	if err := service.Launch(); !errors.Is(err, ErrNotConfigured) {
		t.Fatal(err)
	}
	config := runConfig()
	config.Runtime = coreruntime.OpenCode
	if err := service.Configure(config); err != nil {
		t.Fatal(err)
	}
	if p.calls.Load() != 0 {
		t.Fatal("configuration launched provider")
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			err := service.Launch()
			if err != nil && !errors.Is(err, ErrLaunchConflict) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if p.calls.Load() != 1 {
		t.Fatal("duplicate launch")
	}
	if status := service.Status(); status.State != "ready" || status.SessionID != "session" {
		t.Fatalf("unexpected status: %+v", status)
	}
	_ = p.process.Close()
	if service.Status().State != "exited" {
		t.Fatal("exit not visible")
	}
}

func TestLaunchFailureDoesNotRetry(t *testing.T) {
	p := &fakeRuntimeHost{err: errors.New("launch failed")}
	service := NewRunController(context.Background(), p, "", nil)
	defer func() { _ = service.Close() }()
	config := runConfig()
	config.Runtime = coreruntime.OpenCode
	if err := service.Configure(config); err != nil {
		t.Fatal(err)
	}
	if err := service.Launch(); err == nil {
		t.Fatal("failure hidden")
	}
	if service.Status().State != "failed" {
		t.Fatal("failure state missing")
	}
	if err := service.Launch(); !errors.Is(err, ErrLaunchConflict) {
		t.Fatal("failed attempt retried")
	}
}

func TestUnexpectedProcessExitIsFailed(t *testing.T) {
	process := &fakeRuntimeSession{done: make(chan struct{}), exitErr: errors.New("runtime exited")}
	controller := NewRunController(context.Background(), &fakeRuntimeHost{process: process}, "", nil)
	defer func() { _ = controller.Close() }()
	config := runConfig()
	config.Runtime = coreruntime.OpenCode
	if err := controller.Configure(config); err != nil {
		t.Fatal(err)
	}
	if err := controller.Launch(); err != nil {
		t.Fatal(err)
	}
	if err := process.Close(); err != nil {
		t.Fatal(err)
	}
	if status := controller.Status(); status.State != StateFailed || status.ProcessAlive {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestStartupPersistenceFailureClosesSessionAndBlocksRecovery(t *testing.T) {
	for _, tc := range []struct {
		name    string
		restore bool
	}{{name: "checkpoint"}, {name: "journal replay", restore: true}} {
		t.Run(tc.name, func(t *testing.T) {
			dir, config := t.TempDir(), recoveryConfig()
			if tc.restore {
				journal, _, err := loadJournal(dir, config, false)
				if err != nil {
					t.Fatal(err)
				}
				if err := journal.Close(); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(journal.path, []byte("corrupt\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Mkdir(filepath.Join(dir, "current-run.json"), 0o700); err != nil {
				t.Fatal(err)
			}
			session := &fakeRuntimeSession{done: make(chan struct{})}
			controller := NewRunController(context.Background(), &fakeRuntimeHost{process: session}, dir, nil)
			defer func() { _ = controller.Close() }()
			if err := controller.Configure(config); err != nil {
				t.Fatal(err)
			}
			start := controller.Launch
			if tc.restore {
				start = controller.RestoreSession
			}
			if err := start(); !errors.Is(err, ErrJournal) {
				t.Fatalf("startup did not report persistence failure: %v", err)
			}
			status := controller.Status()
			if status.ProcessAlive || !status.RecoveryBlocked || status.State != StateInterrupted || controller.CanAutoRestore() {
				t.Fatalf("failed startup left execution available: %+v", status)
			}
			select {
			case <-session.Done():
			default:
				t.Fatal("failed startup left its runtime running")
			}
		})
	}
}

func (p *fakeRuntimeHost) Version() string { return "fixture-v1" }
func (p *fakeRuntimeHost) Restore(ctx context.Context, c runconfig.RuntimeConfig) (agentruntime.RuntimeSession, error) {
	return p.Launch(ctx, c)
}

func (p *fakeRuntimeSession) Prompt(context.Context, interaction.Command, agentruntime.PromptObserver) (agentruntime.StopReason, error) {
	return agentruntime.StopReasonEndTurn, nil
}
func (p *fakeRuntimeSession) Cancel(context.Context) error { return nil }
func (p *fakeRuntimeSession) Reconcile(context.Context, interaction.Command) (agentruntime.RecoveryDecision, error) {
	return agentruntime.RecoveryDecision{Action: agentruntime.RecoveryPause}, nil
}
