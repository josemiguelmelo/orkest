package runtimehost

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	agentruntime "github.com/josemiguelmelo/orkest/internal/agentctl/runtime"
)

func TestV1SessionCompatibility(t *testing.T) {
	config := configmodel.RuntimeConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", TaskSessionID: "session", RunID: "revision-2", Runtime: "opencode", Agent: configmodel.AgentDefinition{Name: "Work", Description: "Work", Instructions: "Work"}, Model: configmodel.ModelSelection{Provider: "provider", ID: "model"}}
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
	if err != nil || id.SessionID != "native-v1-session" || id.QuestionToolVersion != 0 {
		t.Fatalf("load legacy snapshot: %+v %v", id, err)
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
	config := configmodel.RuntimeConfig{RunID: "first", MCPServers: []configmodel.MCPServer{{ID: "mcp", URL: "https://example.test", Headers: map[string]string{"Authorization": "first"}}}}
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

func TestRestoredSessionQuestionToolCompatibility(t *testing.T) {
	config := configmodel.RuntimeConfig{OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task"}
	for _, test := range []struct {
		name                                                string
		manifestVersion, savedTool, supportedTool, wantTool int
		incompatible                                        bool
	}{
		{"legacy ignores feature metadata", 3, questionToolVersion, questionToolVersion, 0, false},
		{"current tool", 4, questionToolVersion, questionToolVersion, questionToolVersion, false},
		{"unsupported tool", 4, questionToolVersion + 1, questionToolVersion, 0, true},
		{"profile without tool", 4, questionToolVersion, 0, 0, true},
		{"missing tool version", 4, 0, questionToolVersion, 0, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := sessionStorage{StateDir: t.TempDir(), WorkDir: t.TempDir(), RuntimeVersion: "pinned", QuestionToolVersion: test.supportedTool}
			if err := store.reserveCreation(config); err != nil {
				t.Fatal(err)
			}
			manifest := sessionManifest{Version: test.manifestVersion, QuestionToolVersion: test.savedTool, SessionID: "saved-native-session", RuntimeVersion: store.RuntimeVersion, WorkDir: store.WorkDir, Configuration: sessionConfigurationFingerprint(config)}
			raw, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(store.Root(config), "session.json")
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			restored, err := store.load(config)
			if test.incompatible {
				var failure *sessionManifestCompatibilityError
				if !errors.As(err, &failure) || restored != (restoredSession{}) {
					t.Fatalf("incompatible manifest returned %+v, %v", restored, err)
				}
			} else if err != nil || restored.SessionID != manifest.SessionID || restored.QuestionToolVersion != test.wantTool {
				t.Fatalf("restore returned %+v, %v", restored, err)
			}
			after, err := os.ReadFile(path)
			if err != nil || string(after) != string(raw) {
				t.Fatal("restore changed saved manifest", err)
			}
		})
	}
}

func TestHTTPFingerprintRemainsCompatibleWithOriginalRepresentation(t *testing.T) {
	config := configmodel.RuntimeConfig{MCPServers: []configmodel.MCPServer{{ID: "http", Name: "Remote", URL: "https://example.test"}}}
	before := sessionConfigurationFingerprint(config)
	config.MCPServers[0].Transport = "http"
	if got := sessionConfigurationFingerprint(config); got != before {
		t.Fatal("explicit HTTP broke legacy fingerprint")
	}
}

func TestStdioFingerprintPinsCommandArgumentsAndEnvironmentNames(t *testing.T) {
	config := configmodel.RuntimeConfig{MCPServers: []configmodel.MCPServer{{ID: "stdio", Name: "Tools", Transport: "stdio", Command: "/server", Args: []string{"--original"}, Environment: map[string]string{"TOKEN": "first"}}}}
	before := sessionConfigurationFingerprint(config)
	config.MCPServers[0].Environment["TOKEN"] = "rotated"
	if got := sessionConfigurationFingerprint(config); got != before || config.MCPServers[0].Environment["TOKEN"] != "rotated" {
		t.Fatal("credential rotation changed identity or mutated configuration")
	}
	for _, change := range []func(*configmodel.MCPServer){
		func(server *configmodel.MCPServer) { server.Command = "/other" },
		func(server *configmodel.MCPServer) { server.Args[0] = "--other" },
		func(server *configmodel.MCPServer) { server.Environment["NEW"] = "value" },
	} {
		changed := config.Clone()
		change(&changed.MCPServers[0])
		if sessionConfigurationFingerprint(changed) == before {
			t.Fatal("stdio settings excluded from compatibility")
		}
	}
}

func TestStdioSecretsAreRedactedFromRuntimeActivity(t *testing.T) {
	config := configmodel.RuntimeConfig{MCPServers: []configmodel.MCPServer{{Transport: "stdio", Environment: map[string]string{"TOKEN": "private-stdio-value"}}}}
	var output string
	session := &Session{callbacks: &sessionCallbacks{observer: &agentruntime.PromptObserver{Update: func(update interaction.Update) error { output = update.Text; return nil }}}}
	host := &Host{}
	host.registerSessionSecrets(session, config, preparedRuntime{})
	if err := session.callbacks.Update(interaction.Update{Kind: "text", Text: "token=private-stdio-value"}); err != nil {
		t.Fatal(err)
	}
	if output != "token=[redacted]" {
		t.Fatal("stdio secret exposed in runtime activity")
	}
}
