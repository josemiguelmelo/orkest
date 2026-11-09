package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	runmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	mountmodel "github.com/josemiguelmelo/orkest/internal/server/workspacemount/model"
)

type mountResolver struct {
	mounts []mountmodel.Mount
	calls  int
	err    error
}

func (r *mountResolver) Resolve(context.Context, string) ([]mountmodel.Mount, error) {
	r.calls++
	return slices.Clone(r.mounts), r.err
}

func TestTaskMountSnapshotSurvivesRestoreAndFollowUp(t *testing.T) {
	ctx := context.Background()
	original := []mountmodel.Mount{{Source: "/srv/input", Target: "/mnt/host/input", ReadOnly: true}}
	resolver := &mountResolver{mounts: original}
	repo := &profileSessionRepo{launchRepoFake: &launchRepoFake{}}
	dispatcher, err := NewTaskDispatcher(TaskDispatcherDependencies{
		Agents: &profileAgentGetter{}, Sessions: repo, Launches: &LaunchService{repo: repo},
		Mounts: resolver, Profiles: &profileImageResolver{image: "registry/image@sha256:a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	value := taskmodel.Task{ID: "task", WorkspaceID: "workspace", AssigneeAgentID: "agent", Title: "Work"}
	if err := dispatcher.StartTask(ctx, "user", "org", taskmodel.StartCommand{Task: value}); !errors.Is(err, errReservedForTest) {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(repo.reserved[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &repo.current); err != nil {
		t.Fatal(err)
	}
	repo.exists = true
	resolver.mounts = []mountmodel.Mount{{Source: "/srv/different", Target: "/mnt/host/different"}}
	if err := dispatcher.StartTask(ctx, "user", "org", taskmodel.StartCommand{Task: value, CommandID: "follow-up", PromptText: "Continue"}); !errors.Is(err, errReservedForTest) {
		t.Fatal(err)
	}
	next := repo.reserved[1]
	if !reflect.DeepEqual(next.HostMounts, original) || resolver.calls != 1 || !next.RestoreSession || next.TaskSessionID != repo.current.TaskSessionID || next.InitialCommandID != "follow-up" {
		t.Fatal("follow-up lost pinned mounts or command identity")
	}
	runtimeBytes, err := json.Marshal(next.ForRuntime())
	if err != nil {
		t.Fatal(err)
	}
	var runtimeMap map[string]json.RawMessage
	if err := json.Unmarshal(runtimeBytes, &runtimeMap); err != nil {
		t.Fatal(err)
	}
	if _, exists := runtimeMap["hostMounts"]; exists {
		t.Fatal("executor-only paths leaked into runtime config")
	}
	cloned := next.Clone()
	cloned.HostMounts[0].Source = "/changed"
	if next.HostMounts[0].Source != original[0].Source {
		t.Fatal("clone aliases host mounts")
	}
}

func TestLegacyAndDirectSessionsDoNotAdoptNewMounts(t *testing.T) {
	for _, saved := range [][]mountmodel.Mount{nil, {{Source: "/srv/saved", Target: "/mnt/host/saved", ReadOnly: true}}} {
		config := validConfig()
		config.TaskID, config.TaskSessionID = "conversation", "saved-session"
		config.HostMounts = saved
		store := &directSessionStore{current: config}
		resolver := &mountResolver{mounts: []mountmodel.Mount{{Source: "/srv/new", Target: "/mnt/host/new"}}}
		dispatcher, err := NewTaskDispatcher(TaskDispatcherDependencies{Agents: &profileAgentGetter{}, Sessions: store, Launches: &LaunchService{}, Mounts: resolver})
		if err != nil {
			t.Fatal(err)
		}
		if err := dispatcher.StartDirectChat(context.Background(), "user", "org", "workspace", "agent", "conversation", "follow-up", "Continue"); !errors.Is(err, errReservedForTest) {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(store.reserved.HostMounts, saved) || resolver.calls != 0 || !store.reserved.RestoreSession || store.reserved.InitialPrompt != "Continue" {
			t.Fatal("direct follow-up changed the saved mounts")
		}
	}
	legacy := validConfig()
	legacy.TaskSessionID = "legacy-session"
	repo := &profileSessionRepo{launchRepoFake: &launchRepoFake{}, current: legacy, exists: true}
	dispatcher := &TaskDispatcher{sessions: repo, mounts: &mountResolver{mounts: []mountmodel.Mount{{Source: "/srv/new", Target: "/mnt/host/new"}}}}
	config, found, err := dispatcher.continueTaskSession(context.Background(), "user", "org", taskmodel.Task{ID: "task", WorkspaceID: "workspace"}, "follow-up")
	if err != nil || !found || len(config.HostMounts) != 0 || !config.RestoreSession {
		t.Fatal("legacy task session adopted new mounts", err)
	}
}

func TestFreshDirectSessionResolvesMounts(t *testing.T) {
	resolver := &mountResolver{mounts: []mountmodel.Mount{{Source: "/srv/input", Target: "/mnt/host/input", ReadOnly: true}}}
	dispatcher := &TaskDispatcher{agents: &profileAgentGetter{}, mounts: resolver, profiles: &profileImageResolver{image: "registry/image@sha256:a"}}
	config, err := dispatcher.createDirectSession(context.Background(), "user", "org", "workspace", "agent", "conversation")
	if err != nil || !reflect.DeepEqual(config.HostMounts, resolver.mounts) || resolver.calls != 1 {
		t.Fatal("new direct session did not resolve workspace mounts", err)
	}

	var restored runmodel.RunConfig
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &restored); err != nil || !reflect.DeepEqual(restored.HostMounts, config.HostMounts) {
		t.Fatal("mounts did not survive persistence", err)
	}
}

type mountContinuationStarter struct{ config runmodel.RunConfig }

func (s *mountContinuationStarter) StartAgentctl(_ context.Context, config runmodel.RunConfig) (runmodel.Process, error) {
	s.config = config.Clone()
	return runmodel.Process{ID: "replacement-container", Endpoint: "http://agentctl:8081"}, nil
}

type mountContinuationClient struct {
	agentctlFake
	commandID, prompt string
	config            runmodel.RunConfig
}

func (c *mountContinuationClient) Configure(ctx context.Context, endpoint string, config runmodel.RunConfig) error {
	c.config = config.Clone()
	return c.agentctlFake.Configure(ctx, endpoint, config)
}

func (c *mountContinuationClient) Status(context.Context, string) (runmodel.RunStatus, error) {
	return runmodel.RunStatus{
		OrganizationID: c.config.OrganizationID, WorkspaceID: c.config.WorkspaceID,
		TaskID: c.config.TaskID, RunID: c.config.RunID, TaskSessionID: c.config.TaskSessionID,
		Configured: true, ExecutionAvailable: true, ProcessAlive: true, State: runmodel.StateReady,
	}, nil
}

func (c *mountContinuationClient) RestoreAgent(context.Context, string) error {
	c.calls = append(c.calls, "restore")
	return nil
}

func (c *mountContinuationClient) Prompt(_ context.Context, _, commandID, prompt string) error {
	c.calls = append(c.calls, "prompt")
	c.commandID, c.prompt = commandID, prompt
	return nil
}

type mountCommunication struct{}

func (mountCommunication) Start(context.Context, runmodel.Launch) error { return nil }

func TestRestoredMountConfigurationLaunchesAndAcceptsFollowUp(t *testing.T) {
	for _, mounts := range [][]mountmodel.Mount{
		nil,
		{{Source: "/srv/input", Target: "/mnt/host/input", ReadOnly: true}},
		{{Source: "/srv/aws", Target: "~/.aws", ReadOnly: true}},
	} {
		saved := validConfig()
		saved.TaskSessionID = "persisted-session"
		saved.HostMounts = mounts
		encoded, err := json.Marshal(saved)
		if err != nil {
			t.Fatal(err)
		}
		var restored runmodel.RunConfig
		if err := json.Unmarshal(encoded, &restored); err != nil {
			t.Fatal(err)
		}
		restored.RunID, restored.RestoreSession = "follow-up-run", true
		client := &mountContinuationClient{}
		starter := &mountContinuationStarter{}
		service, err := NewLaunchService(&launchRepoFake{}, starter, client, mountCommunication{}, nil)
		if err != nil {
			t.Fatal(err)
		}
		launch, err := service.StartWithPrompt(context.Background(), restored, "follow-up-command", "Continue the saved work")
		if err != nil {
			t.Fatal(err)
		}
		if launch.Status != runmodel.LaunchReady || launch.TaskSessionID != saved.TaskSessionID ||
			!reflect.DeepEqual(starter.config.HostMounts, mounts) ||
			!reflect.DeepEqual(client.calls, []string{"healthy", "configure", "restore", "prompt"}) ||
			client.commandID != "follow-up-command" || client.prompt != "Continue the saved work" {
			t.Fatal("restored session failed to continue with its pinned mounts")
		}
	}
}
