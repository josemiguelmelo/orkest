package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
)

func TestIntegrationFollowUpRestoresPinnedSession(t *testing.T) {
	repo := &profileSessionRepo{launchRepoFake: &launchRepoFake{}}
	getter := &profileAgentGetter{}
	dispatcher, err := NewTaskDispatcher(TaskDispatcherDependencies{Agents: getter, Launches: &LaunchService{repo: repo}, Sessions: repo})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.profiles = &profileImageResolver{image: "registry/image@sha256:a"}
	value := taskmodel.Task{ID: "task", WorkspaceID: "workspace", AssigneeAgentID: "agent", Title: "Work"}
	if err = dispatcher.StartTask(context.Background(), "user", "org", taskmodel.StartCommand{Task: value, SessionMode: "new"}); !errors.Is(err, errReservedForTest) {
		t.Fatal(err)
	}
	saved := repo.reserved[0]
	// Round-trip platform persistence, then deliver a follow-up with pinned model/configuration.
	payload, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(payload, &repo.current); err != nil {
		t.Fatal(err)
	}
	repo.exists = true
	command := taskmodel.StartCommand{Task: value, CommandID: "review-command", PromptText: "Address review 42", SessionMode: "continue_or_create_if_absent", ExpectedAssigneeID: "agent", GuardAssignment: true}
	if err = dispatcher.StartTask(context.Background(), "user", "org", command); !errors.Is(err, errReservedForTest) {
		t.Fatal(err)
	}
	next := repo.reserved[1]
	if !next.RestoreSession || next.TaskSessionID != saved.TaskSessionID || next.Model != saved.Model || next.Runtime != saved.Runtime || next.Agent != saved.Agent || getter.calls != 1 || !strings.Contains(next.InitialPrompt, "Address review 42") || !next.GuardAssignment || next.ExpectedSessionID != saved.TaskSessionID || next.ExpectedAssigneeID != "agent" {
		t.Fatalf("follow-up lost compatible session: %+v", next)
	}
}

func TestIntegrationFollowUpPreservesUnverifiableOrDifferentAgentSession(t *testing.T) {
	for _, identity := range []string{"", "old-agent"} {
		t.Run("saved-agent-"+identity, func(t *testing.T) {
			repo := &profileSessionRepo{launchRepoFake: &launchRepoFake{}, exists: true, current: orchestrationmodel.RunConfig{AgentDefinitionID: identity, RuntimeConfig: orchestrationmodel.RuntimeConfig{TaskSessionID: "saved-session"}}}
			dispatcher, err := NewTaskDispatcher(TaskDispatcherDependencies{Agents: &profileAgentGetter{}, Launches: &LaunchService{repo: repo}, Sessions: repo})
			if err != nil {
				t.Fatal(err)
			}
			value := taskmodel.Task{ID: "task", WorkspaceID: "workspace", AssigneeAgentID: "agent"}
			err = dispatcher.StartTask(context.Background(), "user", "org", taskmodel.StartCommand{Task: value, PromptText: "Address feedback", ExpectedAssigneeID: "agent", GuardAssignment: true})
			if err == nil || len(repo.reserved) != 0 || repo.current.TaskSessionID != "saved-session" {
				t.Fatal("incompatible session replaced", err)
			}
		})
	}
}

func TestLegacySessionWithoutAgentIdentityStillAcceptsManualFollowUp(t *testing.T) {
	repo := &profileSessionRepo{launchRepoFake: &launchRepoFake{}}
	getter := &profileAgentGetter{}
	dispatcher, err := NewTaskDispatcher(TaskDispatcherDependencies{Agents: getter, Launches: &LaunchService{repo: repo}, Sessions: repo})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.profiles = &profileImageResolver{image: "registry/image@sha256:a"}
	value := taskmodel.Task{ID: "task", WorkspaceID: "workspace", AssigneeAgentID: "agent", Title: "Work"}
	if err = dispatcher.StartTask(context.Background(), "user", "org", taskmodel.StartCommand{Task: value, SessionMode: "new"}); !errors.Is(err, errReservedForTest) {
		t.Fatal(err)
	}
	original := repo.reserved[0]
	original.AgentDefinitionID = ""
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(encoded, &repo.current); err != nil {
		t.Fatal(err)
	}
	repo.exists = true
	if err = dispatcher.StartTask(context.Background(), "user", "org", taskmodel.StartCommand{Task: value, CommandID: "manual-followup", PromptText: "Continue saved work"}); !errors.Is(err, errReservedForTest) {
		t.Fatal(err)
	}
	next := repo.reserved[1]
	if !next.RestoreSession || next.TaskSessionID != original.TaskSessionID || next.Model != original.Model || getter.calls != 1 || !strings.Contains(next.InitialPrompt, "Continue saved work") {
		t.Fatal("legacy manual continuation was changed")
	}
}
