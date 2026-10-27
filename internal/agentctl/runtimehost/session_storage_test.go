package runtimehost

import (
	"os"
	"path/filepath"
	"testing"

	runconfig "github.com/josemiguelmelo/orkest/internal/agentctl/config"
)

func TestV1SessionCompatibility(t *testing.T) {
	config := runconfig.RuntimeConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", TaskSessionID: "session", RunID: "revision-2", Runtime: "opencode", Agent: runconfig.AgentDefinition{Name: "Work", Description: "Work", Instructions: "Work"}, Model: runconfig.ModelSelection{Provider: "provider", ID: "model"}}
	store := sessionStorage{StateDir: t.TempDir(), WorkDir: "/workspace/project", RuntimeVersion: "opencode v2.1.4"}
	if got := configurationFingerprint(config); got != "de63fb245afd0d6666f1f100f13d747de1adce188947918e172ad9d7642ca078" {
		t.Fatalf("v1 fingerprint changed: %s", got)
	}
	want := filepath.Join(store.StateDir, "961e0f4564a3db8eeae39c31bea4d9026bb33bc064377d6252b33e63ee4224e4", "sessions", "3f3af1ecebbd1410ab417ec0d27bbfcb5d340e177ae159b59fc8626c2dfd9175")
	if store.Root(config) != want {
		t.Fatal("v1 storage path changed")
	}
	fixture, err := os.ReadFile("testdata/session-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(want, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(want, "session.json"), fixture, 0o600); err != nil {
		t.Fatal(err)
	}
	id, err := store.load(config)
	if err != nil || id != "native-v1-session" {
		t.Fatalf("load legacy snapshot: %q %v", id, err)
	}
	if err = store.reserveCreation(config); err == nil {
		t.Fatal("fresh launch replaced existing session")
	}
	config.Agent.Instructions = "changed"
	if _, err = store.load(config); err == nil {
		t.Fatal("changed configuration restored")
	}
}

func TestSessionIdentityIgnoresOnlyRunAndCredentials(t *testing.T) {
	config := runconfig.RuntimeConfig{RunID: "first", MCPServers: []runconfig.MCPServer{{ID: "mcp", URL: "https://example.test", Headers: map[string]string{"Authorization": "first"}}}}
	first := configurationFingerprint(config)
	config.RunID = "second"
	config.MCPServers[0].Headers["Authorization"] = "second"
	if configurationFingerprint(config) != first {
		t.Fatal("credential rotation invalidated session")
	}
	if config.MCPServers[0].Headers["Authorization"] != "second" {
		t.Fatal("fingerprinting mutated configuration")
	}
	config.MCPServers[0].URL = "https://other.test"
	if configurationFingerprint(config) == first {
		t.Fatal("MCP destination excluded from compatibility")
	}
}
