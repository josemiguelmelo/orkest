package model

import (
	"testing"

	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
)

func TestClaudeCodeRequiresAnthropicModelProvider(t *testing.T) {
	config := RuntimeConfig{
		OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run",
		Runtime: coreruntime.ClaudeCode, Agent: AgentDefinition{Name: "Work", Instructions: "Work"},
		Model: ModelSelection{Provider: "openai", ID: "claude-sonnet"},
	}
	if err := config.Validate(); err == nil {
		t.Fatal("accepted a non-Anthropic provider for Claude Code")
	}
	config.Model.Provider = "anthropic"
	if err := config.Validate(); err != nil {
		t.Fatalf("rejected Anthropic provider: %v", err)
	}
}
