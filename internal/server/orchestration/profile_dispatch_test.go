package orchestration

import (
	"context"
	"errors"
	"strings"
	"testing"

	agentmodel "github.com/josemiguelmelo/orkest/internal/server/agent/model"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
)

var errReservedForTest = errors.New("reserved")

type profileSessionRepo struct {
	*launchRepoFake
	current  orchestrationmodel.RunConfig
	exists   bool
	reserved []orchestrationmodel.RunConfig
}

func (r *profileSessionRepo) CurrentSession(context.Context, string, string, string) (orchestrationmodel.RunConfig, bool, error) {
	return r.current, r.exists, nil
}

func (r *profileSessionRepo) ReserveSessionRun(_ context.Context, c orchestrationmodel.RunConfig, _ int64) error {
	r.reserved = append(r.reserved, c)
	return errReservedForTest
}

type profileAgentGetter struct{ calls int }

func (g *profileAgentGetter) Get(context.Context, string, string, string, string) (agentmodel.Agent, error) {
	g.calls++
	return agentmodel.Agent{AgentDefinition: agentmodel.AgentDefinition{Name: "Backend", Runtime: "opencode", Provider: "test", Model: "model", ExecutionProfileID: "java"}}, nil
}

type profileImageResolver struct {
	image string
	calls int
}

func (r *profileImageResolver) Resolve(_ context.Context, ws, id string, runtime agentmodel.Runtime) (string, error) {
	r.calls++
	if ws != "workspace" || id != "java" || runtime != "opencode" {
		return "", errors.New("incorrect binding")
	}
	return r.image, nil
}

func TestNextRunAdoptsUpdatedProfileAndRetainsSession(t *testing.T) {
	repo := &profileSessionRepo{launchRepoFake: &launchRepoFake{}}
	getter := &profileAgentGetter{}
	resolver := &profileImageResolver{image: "registry/image@sha256:a"}
	dispatcher, err := NewTaskDispatcher(TaskDispatcherDependencies{Agents: getter, Launches: &LaunchService{repo: repo}, Sessions: repo})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.profiles = resolver
	value := taskmodel.Task{ID: "task", WorkspaceID: "workspace", AssigneeAgentID: "agent", Title: "Work"}
	if err = dispatcher.StartTask(context.Background(), "user", "org", taskmodel.StartCommand{Task: value}); !errors.Is(err, errReservedForTest) {
		t.Fatal(err)
	}
	original := repo.reserved[0]
	repo.current = original
	repo.exists = true
	resolver.image = "registry/image@sha256:b"
	if err = dispatcher.StartTask(context.Background(), "user", "org", taskmodel.StartCommand{Task: value}); !errors.Is(err, errReservedForTest) {
		t.Fatal(err)
	}
	next := repo.reserved[1]
	if original.ExecutionImage == next.ExecutionImage || next.TaskSessionID != original.TaskSessionID || next.RunID == original.RunID || !next.RestoreSession {
		t.Fatal("incorrect session/image transition", original, next)
	}
	if getter.calls != 1 {
		t.Fatal("continuation reread changed agent configuration")
	}
	if repo.current.ExecutionImage != "registry/image@sha256:a" {
		t.Fatal("existing run configuration mutated")
	}
}

func TestTaskContextIsSentOnceAndStageChangesAreFocused(t *testing.T) {
	repo := &profileSessionRepo{launchRepoFake: &launchRepoFake{}}
	getter := &profileAgentGetter{}
	dispatcher, err := NewTaskDispatcher(TaskDispatcherDependencies{Agents: getter, Launches: &LaunchService{repo: repo}, Sessions: repo})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.profiles = &profileImageResolver{image: "registry/image@sha256:a"}
	value := taskmodel.Task{ID: "task", WorkspaceID: "workspace", AssigneeAgentID: "agent", Title: "Task details", StageCustomPrompt: "Create a detailed plan"}
	for i, instruction := range []string{"Create a detailed plan", "Implement the plan"} {
		value.StageCustomPrompt = instruction
		err = dispatcher.StartTask(context.Background(), "user", "org", taskmodel.StartCommand{Task: value, PromptText: "Focus on correctness"})
		if !errors.Is(err, errReservedForTest) {
			t.Fatal(err)
		}
		config := repo.reserved[len(repo.reserved)-1]
		for _, want := range []string{instruction, "Focus on correctness"} {
			if !strings.Contains(config.InitialPrompt, want) {
				t.Fatalf("missing %q in startup prompt", want)
			}
		}
		if i == 0 && (!strings.Contains(config.InitialPrompt, "Task details") || !strings.Contains(config.InitialPrompt, "User startup message:")) {
			t.Fatal("first run did not initialize task context")
		}
		if i == 1 && (strings.Contains(config.InitialPrompt, "Task details") || !strings.Contains(config.InitialPrompt, "Task context update (stage):")) {
			t.Fatal("continuation repeated unchanged context or omitted stage update", config.InitialPrompt)
		}
		repo.current, repo.exists = config, true
	}
	if !repo.reserved[1].RestoreSession || getter.calls != 1 {
		t.Fatal("continuation did not retain configuration")
	}
	if !strings.Contains(repo.reserved[0].InitialPrompt, "Create a detailed plan") {
		t.Fatal("saved prompt changed")
	}
	value.StageCustomPrompt = "Implement the plan"
	if err = dispatcher.StartTask(context.Background(), "user", "org", taskmodel.StartCommand{Task: value, PromptText: "Only the next message"}); !errors.Is(err, errReservedForTest) {
		t.Fatal(err)
	}
	if got := repo.reserved[2].InitialPrompt; got != "Only the next message" {
		t.Fatalf("unchanged context was appended to follow-up: %q", got)
	}
	value.StageCustomPrompt = strings.Repeat("x", orchestrationmodel.MaxPromptBytes)
	if err = dispatcher.StartTask(context.Background(), "user", "org", taskmodel.StartCommand{Task: value}); !errors.Is(err, orchestrationmodel.ErrCommand) {
		t.Fatal("oversized composed prompt accepted", err)
	}
	if len(repo.reserved) != 3 {
		t.Fatal("oversized prompt reserved a run")
	}
}
