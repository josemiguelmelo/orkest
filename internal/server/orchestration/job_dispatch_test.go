package orchestration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	repomodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
	runmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	jobmodel "github.com/josemiguelmelo/orkest/internal/server/scheduledjob/model"
)

type jobDispatchStore struct{ reserved []runmodel.RunConfig }

func (s *jobDispatchStore) ReserveJobSessionRun(_ context.Context, _ jobmodel.Run, c runmodel.RunConfig) error {
	s.reserved = append(s.reserved, c)
	return errReservedForTest
}

func (*jobDispatchStore) TaskCommand(context.Context, string, string, string, string, string) (bool, error) {
	return false, nil
}

func (*jobDispatchStore) MarkJobTimeout(context.Context, string) error { return nil }

type jobRepositories struct{}

func (jobRepositories) List(context.Context, string, string, string) ([]repomodel.CodeRepository, error) {
	return nil, nil
}

func TestJobOccurrencesUseFreshIsolatedSessions(t *testing.T) {
	getter := &profileAgentGetter{}
	store := &jobDispatchStore{}
	helper := &TaskDispatcher{agents: getter, profiles: &profileImageResolver{image: "image"}, repositories: jobRepositories{}}
	dispatcher := NewJobDispatcher(helper, store, nil)
	for _, id := range []string{"first", "second"} {
		run := jobmodel.Run{ID: id, RunID: id + ":run", OrganizationID: "org", WorkspaceID: "workspace", ActorID: "user", CommandID: id + ":start", ScheduledFor: time.Now(), Input: jobmodel.Input{Name: "job", Instructions: "clean mail", AgentID: "agent"}}
		if err := dispatcher.StartJob(context.Background(), run); !errors.Is(err, errReservedForTest) {
			t.Fatal(err)
		}
	}
	if len(store.reserved) != 2 || store.reserved[0].TaskSessionID == store.reserved[1].TaskSessionID {
		t.Fatal("occurrences shared a session")
	}
	for _, config := range store.reserved {
		if config.RestoreSession || config.TaskContext != nil || len(config.MCPServers) > 0 || !strings.Contains(config.InitialPrompt, "clean mail") {
			t.Fatal("job inherited task or conversation state")
		}
	}
}
