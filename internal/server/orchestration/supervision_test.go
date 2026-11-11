package orchestration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/contracts/mcptransport"
	mcpservermodel "github.com/josemiguelmelo/orkest/internal/server/mcpserver/model"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

type supervisedClient struct {
	AgentConnectionClient
	ready       bool
	entered     chan struct{}
	exited      chan struct{}
	connections atomic.Int32
}

func (c *supervisedClient) Connect(ctx context.Context, _ string, _ uint64, ready func(), _ func(orchestrationmodel.AgentActivityEvent) error) error {
	c.connections.Add(1)
	if c.entered != nil {
		close(c.entered)
	}
	defer close(c.exited)
	if c.ready {
		ready()
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestConnectionReadinessTimeoutJoinsAttempt(t *testing.T) {
	client := &supervisedClient{exited: make(chan struct{})}
	service, err := NewConnectionService(context.Background(), client, &launchRepoFake{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	service.readinessTimeout = 20 * time.Millisecond
	err = service.Start(context.Background(), orchestrationmodel.Launch{RunID: "run", Endpoint: "http://worker:8081"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("readiness error = %v", err)
	}
	select {
	case <-client.exited:
	default:
		t.Fatal("startup returned before connection exited")
	}
}

func TestReadyConnectionSurvivesStartupDeadlineAndDeduplicates(t *testing.T) {
	client := &supervisedClient{ready: true, exited: make(chan struct{})}
	service, err := NewConnectionService(context.Background(), client, &launchRepoFake{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	service.readinessTimeout = 20 * time.Millisecond
	launch := orchestrationmodel.Launch{RunID: "run", Endpoint: "http://worker:8081"}
	ctx, cancel := context.WithCancel(context.Background())
	if err = service.Start(ctx, launch); err != nil {
		t.Fatal(err)
	}
	cancel() // Accepted supervision belongs to the platform.
	var work sync.WaitGroup
	for range 20 {
		work.Go(func() {
			if startErr := service.Start(context.Background(), launch); startErr != nil {
				t.Error(startErr)
			}
		})
	}
	work.Wait()
	select {
	case <-client.exited:
		t.Fatal("accepted stream stopped with requesting context or readiness deadline")
	case <-time.After(3 * service.readinessTimeout):
	}
	if n := client.connections.Load(); n != 1 {
		t.Fatalf("connections = %d", n)
	}
	service.Close()
	select {
	case <-client.exited:
	default:
		t.Fatal("Close did not join connection")
	}
	if err = service.Start(context.Background(), launch); !errors.Is(err, context.Canceled) {
		t.Fatalf("start after close = %v", err)
	}
}

func TestConnectionCallerCancellationJoinsUnreadyAttempt(t *testing.T) {
	client := &supervisedClient{entered: make(chan struct{}), exited: make(chan struct{})}
	service, err := NewConnectionService(context.Background(), client, &launchRepoFake{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- service.Start(ctx, orchestrationmodel.Launch{RunID: "run", Endpoint: "http://worker:8081"})
	}()
	<-client.entered
	cancel()
	if err = <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("start = %v", err)
	}
	select {
	case <-client.exited:
	default:
		t.Fatal("cancellation did not join connection")
	}
}

func TestBackgroundCloseGatesAdmissionAndJoins(t *testing.T) {
	owner := newBackgroundWork(context.Background())
	entered, release, returned := make(chan struct{}), make(chan struct{}), make(chan struct{})
	owner.Go(func(ctx context.Context) { close(entered); <-ctx.Done(); <-release })
	<-entered
	go func() { owner.Close(); close(returned) }()
	<-owner.ctx.Done()
	if owner.Go(func(context.Context) { t.Error("work admitted after shutdown") }) {
		t.Fatal("admitted work after shutdown")
	}
	select {
	case <-returned:
		t.Fatal("Close failed to join accepted work")
	default:
	}
	close(release)
	<-returned
	owner.Close()
}

func TestReconcileBatchBoundsConcurrentAttempts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	launches := make([]orchestrationmodel.Launch, 100)
	for i := range launches {
		launches[i].RunID = fmt.Sprint(i)
	}
	var active, maximum atomic.Int32
	entered := make(chan struct{}, 100)
	release, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		reconcileBatch(ctx, launches, func(ctx context.Context, _ orchestrationmodel.Launch) {
			n := active.Add(1)
			defer active.Add(-1)
			for old := maximum.Load(); n > old; old = maximum.Load() {
				if maximum.CompareAndSwap(old, n) {
					break
				}
			}
			entered <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
			}
		})
	}()
	for range 4 {
		<-entered
	}
	if maximum.Load() != 4 {
		t.Fatal("expected four bounded attempts")
	}
	cancel()
	close(release)
	<-done
	if maximum.Load() > 4 || active.Load() != 0 {
		t.Fatalf("max=%d active=%d", maximum.Load(), active.Load())
	}
}

func TestRefreshMCPHeadersIsAtomicAndPreservesSavedSettings(t *testing.T) {
	saved := []orchestrationmodel.MCPServer{{ID: "one", URL: "https://saved", Headers: map[string]string{"token": "old"}, Transport: mcptransport.HTTP}, {ID: "two", Transport: mcptransport.HTTP}}
	resolved := []mcpservermodel.RuntimeServer{{ID: "one", URL: "https://changed", Headers: map[string]string{"token": "fresh"}, Transport: mcptransport.HTTP}}
	if _, err := refreshSessionMCPHeaders(saved, resolved); err == nil {
		t.Fatal("missing server accepted")
	}
	if saved[0].Headers["token"] != "old" {
		t.Fatal("partially changed saved headers")
	}
	resolved = append(resolved, mcpservermodel.RuntimeServer{ID: "two", Transport: mcptransport.HTTP})
	updated, err := refreshSessionMCPHeaders(saved, resolved)
	if err != nil {
		t.Fatal(err)
	}
	resolved[0].Headers["token"] = "later"
	if updated[0].URL != "https://saved" || updated[0].Headers["token"] != "fresh" {
		t.Fatal("settings changed or headers aliased")
	}
	if _, err = refreshSessionMCPHeaders(saved, append(resolved, resolved[0])); err == nil {
		t.Fatal("duplicate resolution accepted")
	}
}

type blockingCleanupController struct {
	sessionProcessController
	entered chan context.Context
	release chan struct{}
}

func (c *blockingCleanupController) TerminateAgentctl(ctx context.Context, _ string) error {
	c.entered <- ctx
	<-c.release
	return nil
}

func TestCompletionShutdownJoinsBoundedTerminalCleanup(t *testing.T) {
	repo := &launchRepoFake{reconnect: []orchestrationmodel.Launch{{RunID: "terminal", ProcessID: "worker", Status: orchestrationmodel.LaunchCompletedPendingClose}}}
	controller := &blockingCleanupController{entered: make(chan context.Context, 1), release: make(chan struct{})}
	service, err := NewLaunchCompletionService(context.Background(), repo, controller)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err = service.RecoverPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	var cleanupCtx context.Context
	select {
	case cleanupCtx = <-controller.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("terminal cleanup did not start")
	}
	returned := make(chan struct{})
	go func() { service.Close(); close(returned) }()
	<-service.background.ctx.Done()
	if cleanupCtx.Err() != nil {
		t.Fatal("shutdown canceled already accepted terminal cleanup")
	}
	select {
	case <-returned:
		t.Fatal("cleanup dependency could close before terminal persistence")
	default:
	}
	close(controller.release)
	<-returned
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.reconnect[0].Status != orchestrationmodel.LaunchCompleted {
		t.Fatal("shutdown did not join terminal persistence")
	}
}

func TestConnectionCloseRacesWithAdmission(t *testing.T) {
	client := &supervisedClient{ready: true, exited: make(chan struct{})}
	service, err := NewConnectionService(context.Background(), client, &launchRepoFake{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	launch := orchestrationmodel.Launch{RunID: "run", Endpoint: "http://worker:8081"}
	if err = service.Start(context.Background(), launch); err != nil {
		t.Fatal(err)
	}
	var work sync.WaitGroup
	for range 20 {
		work.Go(func() { _ = service.Start(context.Background(), launch) })
		work.Go(service.Close)
	}
	work.Wait()
	if client.connections.Load() != 1 {
		t.Fatal("duplicate supervision during shutdown")
	}
}

type cappedHandshakeClient struct {
	AgentConnectionClient
	entered chan struct{}
	active  atomic.Int32
	maximum atomic.Int32
}

func (c *cappedHandshakeClient) Connect(ctx context.Context, _ string, _ uint64, _ func(), _ func(orchestrationmodel.AgentActivityEvent) error) error {
	n := c.active.Add(1)
	defer c.active.Add(-1)
	for old := c.maximum.Load(); n > old; old = c.maximum.Load() {
		if c.maximum.CompareAndSwap(old, n) {
			break
		}
	}
	c.entered <- struct{}{}
	<-ctx.Done()
	return ctx.Err()
}

func TestUnreadyHandshakesAreBoundedAndJoined(t *testing.T) {
	client := &cappedHandshakeClient{entered: make(chan struct{}, 20)}
	service, err := NewConnectionService(context.Background(), client, &launchRepoFake{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	var callers sync.WaitGroup
	for i := range 20 {
		callers.Go(func() {
			_ = service.Start(context.Background(), orchestrationmodel.Launch{RunID: fmt.Sprint(i), Endpoint: "http://worker:8081"})
		})
	}
	for range 4 {
		select {
		case <-client.entered:
		case <-time.After(time.Second):
			t.Fatal("handshakes did not start")
		}
	}
	if client.active.Load() != 4 {
		t.Fatal("expected four occupied handshake slots")
	}
	service.Close()
	callers.Wait()
	if client.maximum.Load() > 4 || client.active.Load() != 0 {
		t.Fatal("handshakes exceeded bound or were not joined")
	}
}
