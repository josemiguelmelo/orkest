package orchestration

import (
	"errors"
	"reflect"
	"testing"

	mcpmodel "github.com/josemiguelmelo/orkest/internal/server/mcpserver/model"
	runmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

type sessionToolResolver func(string, string, string, string, string) (mcpmodel.RuntimeServer, error)

func (f sessionToolResolver) RuntimeServerForRun(userID, orgID, workspaceID, taskID, runID string) (mcpmodel.RuntimeServer, error) {
	return f(userID, orgID, workspaceID, taskID, runID)
}

func TestRefreshOrkestMCPServerPreservesExternalServersAndOrder(t *testing.T) {
	external := runmodel.MCPServer{ID: "external", Name: "External", URL: "https://saved.test", Headers: map[string]string{"Authorization": "external-token"}}
	old := runmodel.MCPServer{ID: runmodel.OrkestMCPServerID, Name: "Old tools", URL: "https://old.test"}
	replacement := runmodel.MCPServer{ID: runmodel.OrkestMCPServerID, Name: "Tools", URL: "https://tools.test", Headers: map[string]string{"Authorization": "run-token"}}
	dispatcher := &TaskDispatcher{agentTools: sessionToolResolver(func(userID, orgID, workspaceID, taskID, runID string) (mcpmodel.RuntimeServer, error) {
		if userID != "user" || orgID != "org" || workspaceID != "workspace" || taskID != "task" || runID != "run" {
			t.Fatal("tool credentials requested for an incorrect scope")
		}
		return mcpmodel.RuntimeServer{ID: replacement.ID, Name: replacement.Name, URL: replacement.URL, Headers: replacement.Headers}, nil
	})}
	for _, test := range []struct {
		name        string
		saved, want []runmodel.MCPServer
	}{
		{name: "empty", want: []runmodel.MCPServer{replacement}},
		{name: "append", saved: []runmodel.MCPServer{external}, want: []runmodel.MCPServer{external, replacement}},
		{name: "replace", saved: []runmodel.MCPServer{old, external}, want: []runmodel.MCPServer{replacement, external}},
		{name: "deduplicate", saved: []runmodel.MCPServer{old, external, old}, want: []runmodel.MCPServer{replacement, external}},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := runmodel.RunConfig{RuntimeConfig: runmodel.RuntimeConfig{RunID: "run", MCPServers: test.saved}}
			snapshot := config.Clone()
			if err := dispatcher.refreshOrkestMCPServer("user", "org", "workspace", "task", &config); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(config.MCPServers, test.want) {
				t.Fatal("tool refresh changed external settings, order, or retained duplicate tools")
			}
			if !reflect.DeepEqual(test.saved, snapshot.MCPServers) {
				t.Fatal("tool refresh mutated saved configuration")
			}
		})
	}
}

func TestRefreshOrkestMCPServerFailurePreservesConfiguration(t *testing.T) {
	config := runmodel.RunConfig{RuntimeConfig: runmodel.RuntimeConfig{MCPServers: []runmodel.MCPServer{{ID: runmodel.OrkestMCPServerID, URL: "https://saved.test"}}}}
	snapshot := config.Clone()
	cause := errors.New("tool credentials unavailable")
	dispatcher := &TaskDispatcher{agentTools: sessionToolResolver(func(string, string, string, string, string) (mcpmodel.RuntimeServer, error) {
		return mcpmodel.RuntimeServer{}, cause
	})}
	if err := dispatcher.refreshOrkestMCPServer("user", "org", "workspace", "task", &config); !errors.Is(err, cause) {
		t.Fatal("tool refresh lost the credential error")
	}
	if !reflect.DeepEqual(config, snapshot) {
		t.Fatal("failed tool refresh changed saved configuration")
	}
}
