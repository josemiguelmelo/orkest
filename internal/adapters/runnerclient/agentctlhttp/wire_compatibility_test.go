package agentctlhttp

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

// The fixture is the pre-refactor v1 configuration from the durable checkpoint.
func TestWireConfigurationMatchesV1Fixture(t *testing.T) {
	fixture, err := os.ReadFile("testdata/configure-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	config := orchestrationmodel.RunConfig{RuntimeConfig: orchestrationmodel.RuntimeConfig{
		OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", TaskSessionID: "conversation", RunID: "run", Runtime: "opencode",
		Agent: orchestrationmodel.AgentDefinition{Name: "Work", Description: "Work", Instructions: "Complete the task."}, Model: orchestrationmodel.ModelSelection{Provider: "provider", ID: "model"},
	}, Repositories: []orchestrationmodel.WorkspaceRepository{{ID: "must-not-leak"}}, InitialPrompt: "must-not-leak"}
	got, err := json.Marshal(toWireRunConfig(config))
	if err != nil {
		t.Fatal(err)
	}
	var actual, previous any
	if err := json.Unmarshal(got, &actual); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(fixture, &previous); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, previous) {
		t.Fatalf("wire configuration changed: %s", got)
	}
}
