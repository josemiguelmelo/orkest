package agentctl

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

type testProvider struct {
	calls   atomic.Int32
	err     error
	process *testProcess
}

func (p *testProvider) Runtime() string { return "alternative" }
func (p *testProvider) Launch(context.Context, RunConfig) (Process, error) {
	p.calls.Add(1)
	return p.process, p.err
}

type testProcess struct {
	done    chan struct{}
	once    sync.Once
	exitErr error
}

func (p *testProcess) Err() error {
	select {
	case <-p.done:
		return p.exitErr
	default:
		return nil
	}
}
func (p *testProcess) SessionID() string     { return "session" }
func (p *testProcess) Done() <-chan struct{} { return p.done }
func (p *testProcess) Close() error          { p.once.Do(func() { close(p.done) }); return nil }

func TestProviderLaunchOwnership(t *testing.T) {
	p := &testProvider{process: &testProcess{done: make(chan struct{})}}
	service := NewRunControllerWithProvider(context.Background(), p, "")
	defer func() { _ = service.Close() }()
	if err := service.Launch(); !errors.Is(err, ErrNotConfigured) {
		t.Fatal(err)
	}
	config := runConfig()
	config.Runtime = "alternative"
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
	p := &testProvider{err: errors.New("launch failed")}
	service := NewRunControllerWithProvider(context.Background(), p, "")
	defer func() { _ = service.Close() }()
	config := runConfig()
	config.Runtime = "alternative"
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
	process := &testProcess{done: make(chan struct{}), exitErr: errors.New("runtime exited")}
	controller := NewRunControllerWithProvider(context.Background(), &testProvider{process: process}, "")
	defer func() { _ = controller.Close() }()
	config := runConfig()
	config.Runtime = "alternative"
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
