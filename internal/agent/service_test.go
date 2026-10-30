package agent

import (
	"context"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/workspace"
)

type repositoryStub struct{ value Agent }

func (r *repositoryStub) Agents(context.Context, string, string) ([]Agent, error) {
	return []Agent{r.value}, nil
}

func (r *repositoryStub) Agent(context.Context, string, string, string) (Agent, error) {
	return r.value, nil
}

func (r *repositoryStub) CreateAgent(_ context.Context, id, workspaceID string, definition AgentDefinition) (Agent, error) {
	r.value = Agent{ID: id, WorkspaceID: workspaceID, AgentDefinition: definition}
	return r.value, nil
}

func (r *repositoryStub) UpdateAgent(_ context.Context, _, workspaceID, id string, definition AgentDefinition) (Agent, error) {
	r.value = Agent{ID: id, WorkspaceID: workspaceID, AgentDefinition: definition}
	return r.value, nil
}
func (*repositoryStub) DeleteAgent(context.Context, string, string, string) error { return nil }

type workspaceStub struct{ role string }

func (w workspaceStub) GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error) {
	return workspace.Workspace{Role: w.role}, nil
}

type mcpAccessStub struct{ valid bool }

func (m mcpAccessStub) Exists(context.Context, string, string, []string) (bool, error) {
	return m.valid, nil
}

func validDefinition() AgentDefinition {
	return AgentDefinition{Name: "Work", Runtime: RuntimeOpenCode, Provider: "provider", Model: "model", Description: "work"}
}

func TestCreateRejectsMCPServerOutsideWorkspace(t *testing.T) {
	service := NewService(&repositoryStub{}, workspaceStub{role: "member"}, mcpAccessStub{valid: false}, nil)
	definition := validDefinition()
	definition.MCPServerIDs = []string{"server-from-another-workspace"}
	if _, err := service.Create(context.Background(), "user", "org", "workspace", definition); err != modelerrors.ErrInvalid {
		t.Fatalf("expected invalid MCP selection, got %v", err)
	}
}

func TestUpdatePreservesMCPSelectionWhenFieldOmitted(t *testing.T) {
	repository := &repositoryStub{value: Agent{ID: "agent", WorkspaceID: "workspace", AgentDefinition: validDefinition()}}
	repository.value.MCPServerIDs = []string{"selected-server"}
	service := NewService(repository, workspaceStub{role: "member"}, mcpAccessStub{valid: true}, nil)
	updated, err := service.Update(context.Background(), "user", "org", "workspace", "agent", validDefinition())
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.MCPServerIDs) != 1 || updated.MCPServerIDs[0] != "selected-server" {
		t.Fatalf("MCP selection was lost: %#v", updated.MCPServerIDs)
	}
}

func TestDefinitionValidationDelegatesToRuntimeProvider(t *testing.T) {
	definition := validDefinition()
	definition.Runtime = RuntimeClaudeCode
	definition.Provider = "openai"
	service := NewService(&repositoryStub{}, workspaceStub{role: "member"}, mcpAccessStub{valid: true}, nil)
	if err := service.validateDefinition(&definition); err != modelerrors.ErrInvalid {
		t.Fatalf("expected runtime provider validation error, got %v", err)
	}
	if err := validate(&definition); err != nil {
		t.Fatalf("generic validation should not know runtime-specific rules: %v", err)
	}
}
