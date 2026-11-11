package orchestration

import (
	"context"
	"errors"
	"testing"

	mcpmodel "github.com/josemiguelmelo/orkest/internal/server/mcpserver/model"
	runmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

type directSessionStore struct {
	TaskSessionStore
	current  runmodel.RunConfig
	reserved runmodel.RunConfig
}

func (s *directSessionStore) CurrentDirectSession(context.Context, string, string, string) (runmodel.RunConfig, bool, error) {
	return s.current.Clone(), true, nil
}

func (s *directSessionStore) ReserveDirectSessionRun(_ context.Context, config runmodel.RunConfig) error {
	s.reserved = config
	return errReservedForTest
}

type sessionMCPResolver struct{ calls int }

func (r *sessionMCPResolver) Resolve(_ context.Context, workspaceID string, ids []string) ([]mcpmodel.RuntimeServer, error) {
	r.calls++
	if workspaceID != "workspace" || len(ids) != 1 || ids[0] != "external" {
		return nil, errors.New("unexpected credential scope")
	}
	return []mcpmodel.RuntimeServer{{ID: "external", URL: "https://changed.test", Headers: map[string]string{"Authorization": "fresh"}, Transport: "http"}}, nil
}

func TestDirectContinuationPreservesPinnedConfigurationBeforeReservation(t *testing.T) {
	for _, external := range []bool{false, true} {
		config := validConfig()
		config.TaskID, config.TaskSessionID = "conversation", "saved-session"
		config.MCPServers = []runmodel.MCPServer{{ID: runmodel.OrkestMCPServerID, Name: "Tools", URL: "https://tools.test", Transport: "http"}}
		if external {
			config.MCPServers = append(config.MCPServers, runmodel.MCPServer{ID: "external", Name: "External", URL: "https://saved.test", Headers: map[string]string{"Authorization": "old"}, Transport: "http"})
		}
		store := &directSessionStore{current: config}
		getter := &profileAgentGetter{}
		dispatcher, err := NewTaskDispatcher(TaskDispatcherDependencies{Agents: getter, Sessions: store, Launches: &LaunchService{}})
		if err != nil {
			t.Fatal(err)
		}
		resolver := &sessionMCPResolver{}
		if external {
			dispatcher.mcpServers = resolver
		}
		err = dispatcher.StartDirectChat(context.Background(), "user", "org", "workspace", "changed-agent", "conversation", "follow-up", "continue")
		if !errors.Is(err, errReservedForTest) {
			t.Fatal(err)
		}
		got := store.reserved
		if getter.calls != 0 || !got.RestoreSession || got.TaskSessionID != config.TaskSessionID || got.Model != config.Model || got.Agent != config.Agent || got.Runtime != config.Runtime || got.RunID == config.RunID || got.InitialCommandID != "follow-up" {
			t.Fatal("continuation changed pinned settings or lost command identity")
		}
		if external && (resolver.calls != 1 || got.MCPServers[1].URL != "https://saved.test" || got.MCPServers[1].Headers["Authorization"] != "fresh" || store.current.MCPServers[1].Headers["Authorization"] != "old") {
			t.Fatal("credential refresh changed saved settings or aliased state")
		}
	}
}
