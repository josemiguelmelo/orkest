package orchestration

import (
	"context"
	"errors"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/agent"
	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
	"github.com/josemiguelmelo/orkest/internal/task"
)

var errReservedForTest = errors.New("reserved")

type profileSessionRepo struct {
	*launchRepoFake
	current  runconfig.RunConfig
	exists   bool
	reserved []runconfig.RunConfig
}

func (r *profileSessionRepo) CurrentSession(context.Context, string, string, string) (runconfig.RunConfig, bool, error) {
	return r.current, r.exists, nil
}

func (r *profileSessionRepo) ReserveSessionRun(_ context.Context, c runconfig.RunConfig, _ int64) error {
	r.reserved = append(r.reserved, c)
	return errReservedForTest
}

type profileAgentGetter struct{ calls int }

func (g *profileAgentGetter) Get(context.Context, string, string, string, string) (agent.Agent, error) {
	g.calls++
	return agent.Agent{AgentDefinition: agent.AgentDefinition{Name: "Backend", Runtime: "opencode", Provider: "test", Model: "model", ExecutionProfileID: "java"}}, nil
}

type profileImageResolver struct {
	image string
	calls int
}

func (r *profileImageResolver) Resolve(_ context.Context, ws, id string, runtime agent.Runtime) (string, error) {
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
	dispatcher, err := NewTaskDispatcher(getter, &LaunchService{repo: repo})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.SetProfiles(resolver)
	value := task.Task{ID: "task", WorkspaceID: "workspace", AssigneeAgentID: "agent", Title: "Work"}
	if err = dispatcher.StartTask(context.Background(), "user", "org", value); !errors.Is(err, errReservedForTest) {
		t.Fatal(err)
	}
	original := repo.reserved[0]
	repo.current = original
	repo.exists = true
	resolver.image = "registry/image@sha256:b"
	if err = dispatcher.StartTask(context.Background(), "user", "org", value); !errors.Is(err, errReservedForTest) {
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
