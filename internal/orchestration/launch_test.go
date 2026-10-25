package orchestration

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
)

func validConfig() agentctl.RunConfig {
	return agentctl.RunConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run", Runtime: "opencode", Model: agentctl.ModelSelection{Provider: "provider", ID: "model"}, SystemPrompt: "work"}
}

type launchRepoFake struct {
	mu       sync.Mutex
	launches []Launch
	events   []Event
}

func (r *launchRepoFake) CreateLaunch(_ context.Context, l Launch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.launches = append(r.launches, l)
	return nil
}

func (r *launchRepoFake) UpdateLaunch(_ context.Context, l Launch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.launches = append(r.launches, l)
	return nil
}

func (r *launchRepoFake) SaveEvent(_ context.Context, e Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
	return nil
}

type launcherFake struct {
	process Process
	err     error
}

func (l launcherFake) LaunchAgentctl(context.Context, agentctl.RunConfig) (Process, error) {
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

func (c *agentctlFake) Configure(context.Context, string, agentctl.RunConfig) error {
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

func (c *agentctlFake) Connect(ctx context.Context, _ string, _ uint64, onConnected func(), onEvent func(agentctl.Event) error) error {
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
	communication, err := NewConnectionService(ctx, client, repo)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewLaunchService(repo, launcherFake{process: Process{ID: "container-1", Endpoint: "http://agentctl:8080"}}, client, communication)
	if err != nil {
		t.Fatal(err)
	}
	launch, err := service.Start(ctx, validConfig())
	if err != nil {
		t.Fatal(err)
	}
	if launch.Status != LaunchReady || launch.ProcessID != "container-1" {
		t.Fatalf("unexpected launch: %+v", launch)
	}
	if got, want := client.calls, []string{"healthy", "configure", "connect", "launch"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	repo.mu.Lock()
	launches := append([]Launch(nil), repo.launches...)
	repo.mu.Unlock()
	if len(launches) != 3 || launches[0].Status != LaunchStarting || launches[1].Status != LaunchProcessStarted || launches[2].Status != LaunchReady {
		t.Fatalf("unexpected persisted lifecycle: %+v", launches)
	}
}

func TestLaunchFailureRetainsProcessIdentityAndSanitizesReason(t *testing.T) {
	repo := &launchRepoFake{}
	client := &agentctlFake{}
	client.configureErr = errors.New("secret endpoint credential")
	communication, err := NewConnectionService(context.Background(), client, repo)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewLaunchService(repo, launcherFake{process: Process{ID: "container-1", Endpoint: "http://agentctl:8080"}}, client, communication)
	if err != nil {
		t.Fatal(err)
	}
	launch, err := service.Start(context.Background(), validConfig())
	if err == nil {
		t.Fatal("expected failure")
	}
	if launch.Status != LaunchFailed || launch.ProcessID != "container-1" || launch.FailureStage != "configure" {
		t.Fatalf("unexpected failed launch: %+v", launch)
	}
	if launch.FailureReason == "" || launch.FailureReason == "secret endpoint credential" {
		t.Fatalf("failure reason was not sanitized: %q", launch.FailureReason)
	}
}

func TestConnectionServicePersistsEventAndReconnectsFromCursor(t *testing.T) {
	repo := &launchRepoFake{}
	client := &eventClientFake{event: agentctl.Event{RunID: "run", Sequence: 7}, cursors: make(chan uint64, 2)}
	ctx, cancel := context.WithCancel(context.Background())
	service, err := NewConnectionService(ctx, client, repo)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Start("run", "http://agentctl:8080", 4); err != nil {
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
	events := append([]Event(nil), repo.events...)
	repo.mu.Unlock()
	if len(events) != 1 || events[0].Sequence != 7 {
		t.Fatalf("persisted events = %+v", events)
	}
}

type eventClientFake struct {
	event    agentctl.Event
	cursors  chan uint64
	connects int
	mu       sync.Mutex
}

func (*eventClientFake) WaitHealthy(context.Context, string) error                   { return nil }
func (*eventClientFake) Configure(context.Context, string, agentctl.RunConfig) error { return nil }
func (*eventClientFake) StartAgent(context.Context, string) error                    { return nil }
func (*eventClientFake) Prompt(context.Context, string, string, string) error        { return nil }
func (c *eventClientFake) Connect(ctx context.Context, _ string, cursor uint64, onConnected func(), onEvent func(agentctl.Event) error) error {
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
