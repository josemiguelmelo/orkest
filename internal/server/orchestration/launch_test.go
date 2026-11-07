package orchestration

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

func validConfig() orchestrationmodel.RunConfig {
	return orchestrationmodel.RunConfig{RuntimeConfig: orchestrationmodel.RuntimeConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run", Runtime: "opencode", Model: orchestrationmodel.ModelSelection{Provider: "provider", ID: "model"}, Agent: orchestrationmodel.AgentDefinition{Name: "Work", Description: "Work", Instructions: "work"}}}
}

type launchRepoFake struct {
	mu        sync.Mutex
	launches  []orchestrationmodel.Launch
	reconnect []orchestrationmodel.Launch
	events    []orchestrationmodel.Event
}

func (r *launchRepoFake) CreateLaunch(_ context.Context, l orchestrationmodel.Launch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.launches = append(r.launches, l)
	return nil
}

func (r *launchRepoFake) UpdateLaunch(_ context.Context, l orchestrationmodel.Launch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.launches = append(r.launches, l)
	return nil
}

func (r *launchRepoFake) ListReconnectableLaunches(context.Context) ([]orchestrationmodel.Launch, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]orchestrationmodel.Launch(nil), r.reconnect...), nil
}

func (r *launchRepoFake) SaveTerminalEvent(ctx context.Context, event orchestrationmodel.Event, status orchestrationmodel.LaunchStatus) error {
	if err := r.SaveEvent(ctx, event); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reconnect = append(r.reconnect, orchestrationmodel.Launch{RunID: event.RunID, Status: status})
	return nil
}

func (r *launchRepoFake) SetLaunchStatus(_ context.Context, effects LaunchStatusEffects) error {
	runID, status := effects.RunID, effects.Status
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.reconnect {
		if r.reconnect[i].RunID == runID {
			r.reconnect[i].Status = status
		}
	}
	return nil
}

func (r *launchRepoFake) SaveEvent(_ context.Context, e orchestrationmodel.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	return nil
}

type launcherFake struct {
	process orchestrationmodel.Process
	err     error
}

func (l launcherFake) StartAgentctl(context.Context, orchestrationmodel.RunConfig) (orchestrationmodel.Process, error) {
	return l.process, l.err
}

type agentctlFake struct {
	calls        []string
	connectErr   error
	configureErr error
}

func (c *agentctlFake) WaitHealthy(context.Context, string) error {
	c.calls = append(c.calls, "healthy")
	return nil
}

func (*agentctlFake) Status(context.Context, string) (orchestrationmodel.RunStatus, error) {
	return orchestrationmodel.RunStatus{Configured: true, ExecutionAvailable: true, ProcessAlive: true, State: orchestrationmodel.StateReady}, nil
}

func (c *agentctlFake) Configure(context.Context, string, orchestrationmodel.RunConfig) error {
	c.calls = append(c.calls, "configure")
	return c.configureErr
}

func (c *agentctlFake) StartAgent(context.Context, string) error {
	c.calls = append(c.calls, "launch")
	return nil
}

func (c *agentctlFake) Prompt(context.Context, string, string, string) error {
	c.calls = append(c.calls, "prompt")
	return nil
}

func (c *agentctlFake) Connect(ctx context.Context, _ string, _ uint64, onConnected func(), onEvent func(orchestrationmodel.AgentActivityEvent) error) error {
	c.calls = append(c.calls, "connect")
	onConnected()
	if c.connectErr != nil {
		return c.connectErr
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestLaunchServicePersistsLifecycleAndStartsStreamBeforeAgent(t *testing.T) {
	repo := &launchRepoFake{}
	client := &agentctlFake{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	communication, err := NewConnectionService(ctx, client, repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewLaunchService(repo, launcherFake{process: orchestrationmodel.Process{ID: "container-1", Endpoint: "http://agentctl:8080"}}, client, communication, nil)
	if err != nil {
		t.Fatal(err)
	}
	launch, err := service.Start(ctx, validConfig())
	if err != nil {
		t.Fatal(err)
	}
	if launch.Status != orchestrationmodel.LaunchReady || launch.ProcessID != "container-1" {
		t.Fatalf("unexpected launch: %+v", launch)
	}
	if got, want := client.calls, []string{"healthy", "configure", "connect", "launch"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	repo.mu.Lock()
	launches := append([]orchestrationmodel.Launch(nil), repo.launches...)
	repo.mu.Unlock()
	if len(launches) != 3 || launches[0].Status != orchestrationmodel.LaunchStarting || launches[1].Status != orchestrationmodel.LaunchProcessStarted || launches[2].Status != orchestrationmodel.LaunchReady {
		t.Fatalf("unexpected persisted lifecycle: %+v", launches)
	}
}

func TestLaunchFailureRetainsProcessIdentityAndSanitizesReason(t *testing.T) {
	repo := &launchRepoFake{}
	client := &agentctlFake{}
	client.configureErr = errors.New("secret endpoint credential")
	communication, err := NewConnectionService(context.Background(), client, repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewLaunchService(repo, launcherFake{process: orchestrationmodel.Process{ID: "container-1", Endpoint: "http://agentctl:8080"}}, client, communication, nil)
	if err != nil {
		t.Fatal(err)
	}
	launch, err := service.Start(context.Background(), validConfig())
	if err == nil {
		t.Fatal("expected failure")
	}
	if launch.Status != orchestrationmodel.LaunchFailed || launch.ProcessID != "container-1" || launch.FailureStage != "configure" {
		t.Fatalf("unexpected failed launch: %+v", launch)
	}
	if launch.FailureReason == "" || launch.FailureReason == "secret endpoint credential" {
		t.Fatalf("failure reason was not sanitized: %q", launch.FailureReason)
	}
}

func TestConnectionServicePersistsEventAndReconnectsFromCursor(t *testing.T) {
	repo := &launchRepoFake{}
	client := &eventClientFake{event: orchestrationmodel.AgentActivityEvent{RunID: "run", Sequence: 7}, cursors: make(chan uint64, 2)}
	ctx, cancel := context.WithCancel(context.Background())
	service, err := NewConnectionService(ctx, client, repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Start(ctx, orchestrationmodel.Launch{RunID: "run", ProcessID: "container-1", Endpoint: "http://agentctl:8080", LastSequence: 4}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-client.cursors:
		if got != 4 {
			t.Fatalf("first cursor = %d, want 4", got)
		}
	case <-time.After(time.Second):
		t.Fatal("initial connection did not start")
	}
	select {
	case got := <-client.cursors:
		if got != 7 {
			t.Fatalf("reconnect cursor = %d, want 7", got)
		}
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("stream did not reconnect")
	}
	cancel()
	repo.mu.Lock()
	events := append([]orchestrationmodel.Event(nil), repo.events...)
	repo.mu.Unlock()
	if len(events) != 1 || events[0].Sequence != 7 {
		t.Fatalf("persisted events = %+v", events)
	}
}

type eventClientFake struct {
	event    orchestrationmodel.AgentActivityEvent
	cursors  chan uint64
	connects int
	mu       sync.Mutex
}

func (*eventClientFake) WaitHealthy(context.Context, string) error { return nil }

func (*eventClientFake) Status(context.Context, string) (orchestrationmodel.RunStatus, error) {
	return orchestrationmodel.RunStatus{Configured: true, ExecutionAvailable: true, ProcessAlive: true, State: orchestrationmodel.StateReady}, nil
}

func (*eventClientFake) Configure(context.Context, string, orchestrationmodel.RunConfig) error {
	return nil
}

func (*eventClientFake) StartAgent(context.Context, string) error { return nil }

func (*eventClientFake) Prompt(context.Context, string, string, string) error { return nil }

func (c *eventClientFake) Connect(ctx context.Context, _ string, cursor uint64, onConnected func(), onEvent func(orchestrationmodel.AgentActivityEvent) error) error {
	c.cursors <- cursor
	onConnected()
	c.mu.Lock()
	c.connects++
	attempt := c.connects
	c.mu.Unlock()
	if attempt == 1 {
		if err := onEvent(c.event); err != nil {
			return err
		}
		return errors.New("disconnected")
	}
	<-ctx.Done()
	return ctx.Err()
}

func (c *agentctlFake) RestoreAgent(context.Context, string) error { return nil }

func (c *agentctlFake) CancelPrompt(context.Context, string, string) error { return nil }

func (c *agentctlFake) RespondPermission(context.Context, string, orchestrationmodel.PermissionResponse) error {
	return nil
}

func (c *agentctlFake) Queue(context.Context, string, bool) error { return nil }

func (r *launchRepoFake) ListTaskLaunches(context.Context, string, string, string) ([]orchestrationmodel.Launch, error) {
	return nil, nil
}

func (r *launchRepoFake) ListTaskLaunchEvents(context.Context, string, uint64) ([]orchestrationmodel.Event, error) {
	return nil, nil
}

func (r *launchRepoFake) CurrentSession(context.Context, string, string, string) (orchestrationmodel.RunConfig, bool, error) {
	return orchestrationmodel.RunConfig{}, false, nil
}

func (r *launchRepoFake) ReserveSessionRun(context.Context, orchestrationmodel.RunConfig, int64) error {
	return nil
}

func (r *launchRepoFake) RecordPrompt(context.Context, string, string, string) (bool, error) {
	return false, nil
}

func (r *launchRepoFake) PromptDelivered(context.Context, string, string) error { return nil }

func (r *launchRepoFake) PendingPrompts(context.Context, string) ([]PromptDelivery, error) {
	return nil, nil
}

func (r *launchRepoFake) TaskCommand(context.Context, string, string, string, string, string) (bool, error) {
	return false, nil
}

func (c *agentctlFake) RespondQuestion(context.Context, string, orchestrationmodel.QuestionResponse) error {
	return nil
}
