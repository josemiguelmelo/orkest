package httpapi

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	"github.com/josemiguelmelo/orkest/internal/contracts/mcptransport"

	wire "github.com/josemiguelmelo/orkest/internal/contracts/agentctl/v1"
)

func TestAcceptPreviousV1Configuration(t *testing.T) {
	fixture, err := os.ReadFile("testdata/configure-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var request wire.RunConfig
	if err := json.Unmarshal(fixture, &request); err != nil {
		t.Fatal(err)
	}
	config := fromWireRunConfig(request)
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	if config.TaskSessionID != "conversation" || config.Model.Provider != "provider" || config.Model.ID != "model" || config.Agent.Instructions != "Complete the task." {
		t.Fatal("v1 configuration lost session configuration")
	}
}

func TestPreviousPermissionEventWireShape(t *testing.T) {
	previous := `{"at":"2026-11-03T00:00:00Z","sequence":42,"type":"permission_requested","organizationId":"org","workspaceId":"workspace","taskId":"task","runId":"run","taskSessionId":"conversation","sessionId":"native","commandId":"command","requestId":"request","permission":{"toolCallId":"tool","title":"Read file","options":[{"id":"allow-once","name":"Allow once","kind":"allow_once"}]}}`
	event := interaction.Event{At: time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC), Sequence: 42, Type: interaction.EventPermissionRequested, OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run", TaskSessionID: "conversation", SessionID: "native", CommandID: "command", RequestID: "request", Permission: &interaction.Permission{ToolCallID: "tool", Title: "Read file", Options: []interaction.PermissionOption{{ID: "allow-once", Name: "Allow once", Kind: "allow_once"}}}}
	encoded, err := json.Marshal(toWireEvent(event))
	if err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err = json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal([]byte(previous), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("permission identity or wire optionality changed: %s", encoded)
	}
}

func TestStdioMCPConfigurationAccepted(t *testing.T) {
	server := fromWireMCPServer(wire.MCPServer{ID: "local", Name: "Local", Transport: mcptransport.Stdio, Command: "/server", Args: []string{"literal space"}, Environment: map[string]string{"TOKEN": "private"}})
	if server.Transport != mcptransport.Stdio || server.Command != "/server" || len(server.Args) != 1 || server.Args[0] != "literal space" || server.Environment["TOKEN"] != "private" || server.URL != "" || len(server.Headers) != 0 {
		t.Fatal("stdio configuration lost on worker wire mapping")
	}
}
