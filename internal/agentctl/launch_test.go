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
func (p *testProvider) Launch(context.Context, Configuration) (Process, error) {
	p.calls.Add(1)
	return p.process, p.err
}

type testProcess struct {
	done chan struct{}
	once sync.Once
}

func (p *testProcess) SessionID() string     { return "session" }
func (p *testProcess) Done() <-chan struct{} { return p.done }
func (p *testProcess) Close() error          { p.once.Do(func() { close(p.done) }); return nil }

func TestProviderLaunchOwnership(t *testing.T) {
	p := &testProvider{process: &testProcess{done: make(chan struct{})}}
	service := NewServiceWithProvider(context.Background(), p)
	defer func() { _ = service.Close() }()
	if err := service.Launch(); !errors.Is(err, ErrNotConfigured) {
		t.Fatal(err)
	}
	config := configuration()
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
	service := NewServiceWithProvider(context.Background(), p)
	defer func() { _ = service.Close() }()
	config := configuration()
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
