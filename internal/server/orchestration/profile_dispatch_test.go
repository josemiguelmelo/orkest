package orchestration

import (
	"context"
	"errors"
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
