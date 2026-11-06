package agent

import (
	"context"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	agentmodel "github.com/josemiguelmelo/orkest/internal/server/agent/model"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type repositoryStub struct{ value agentmodel.Agent }

func (r *repositoryStub) Agents(context.Context, string, string) ([]agentmodel.Agent, error) {
	return []agentmodel.Agent{r.value}, nil
}

func (r *repositoryStub) Agent(context.Context, string, string, string) (agentmodel.Agent, error) {
	return r.value, nil
}

func (r *repositoryStub) CreateAgent(_ context.Context, id, workspaceID string, definition agentmodel.AgentDefinition) (agentmodel.Agent, error) {
	r.value = agentmodel.Agent{ID: id, WorkspaceID: workspaceID, AgentDefinition: definition}
	return r.value, nil
}

func (r *repositoryStub) UpdateAgent(_ context.Context, _, workspaceID, id string, definition agentmodel.AgentDefinition) (agentmodel.Agent, error) {
	r.value = agentmodel.Agent{ID: id, WorkspaceID: workspaceID, AgentDefinition: definition}
	return r.value, nil
}

func (*repositoryStub) DeleteAgent(context.Context, string, string, string) error { return nil }

type workspaceStub struct{ role string }

func (w workspaceStub) GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error) {
	return workspacemodel.Workspace{Role: w.role}, nil
}

type mcpAccessStub struct{ valid bool }

func (m mcpAccessStub) Exists(context.Context, string, string, []string) (bool, error) {
	return m.valid, nil
}

type runtimeProfileStub struct{ runtime agentmodel.Runtime }

func (p runtimeProfileStub) ResolveAgentProfile(context.Context, string, string) (agentmodel.Runtime, string, string, map[string]string, error) {
	return p.runtime, "anthropic", "gateway/claude-sonnet", nil, nil
}

func validDefinition() agentmodel.AgentDefinition {
	return agentmodel.AgentDefinition{Name: "Work", Runtime: agentmodel.RuntimeOpenCode, Provider: "provider", Model: "model", Description: "work"}
}

func TestPermissionPolicyDefaultsToAskAndRejectsUnknownValues(t *testing.T) {
	definition := validDefinition()
	if err := validate(&definition); err != nil {
		t.Fatal(err)
	}
	if definition.PermissionPolicy != agentmodel.PermissionPolicyAsk {
		t.Fatalf("default policy = %q, want ask", definition.PermissionPolicy)
	}
	definition.PermissionPolicy = "unexpected"
	if err := validate(&definition); err != modelerrors.ErrInvalid {
		t.Fatalf("unknown policy error = %v, want invalid", err)
	}
}

func TestCreateRejectsMCPServerOutsideWorkspace(t *testing.T) {
	service := NewAgentDefinitionUseCase(&repositoryStub{}, workspaceStub{role: "member"}, mcpAccessStub{valid: false}, nil, nil, nil)
	definition := validDefinition()
	definition.MCPServerIDs = []string{"server-from-another-workspace"}
	if _, err := service.Create(context.Background(), "user", "org", "workspace", definition); err != modelerrors.ErrInvalid {
		t.Fatalf("expected invalid MCP selection, got %v", err)
	}
}

func TestUpdatePreservesMCPSelectionWhenFieldOmitted(t *testing.T) {
	repository := &repositoryStub{value: agentmodel.Agent{ID: "agent", WorkspaceID: "workspace", AgentDefinition: validDefinition()}}
	repository.value.MCPServerIDs = []string{"selected-server"}
	service := NewAgentDefinitionUseCase(repository, workspaceStub{role: "member"}, mcpAccessStub{valid: true}, nil, nil, nil)
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
	definition.Runtime = agentmodel.RuntimeClaudeCode
	definition.Provider = "openai"
	service := NewAgentDefinitionUseCase(&repositoryStub{}, workspaceStub{role: "member"}, mcpAccessStub{valid: true}, nil, nil, nil)
	if err := service.validateDefinition(&definition); err != modelerrors.ErrInvalid {
		t.Fatalf("expected runtime provider validation error, got %v", err)
	}
	if err := validate(&definition); err != nil {
		t.Fatalf("generic validation should not know runtime-specific rules: %v", err)
	}
}

func TestCreateResolvesRuntimeProfileSettings(t *testing.T) {
	repository := &repositoryStub{}
	service := NewAgentDefinitionUseCase(repository, workspaceStub{role: "member"}, mcpAccessStub{valid: true}, nil, nil, nil)
	service.runtimeProfiles = runtimeProfileStub{runtime: agentmodel.RuntimeClaudeCode}
	definition := validDefinition()
	definition.RuntimeProfileID = "profile"
	created, err := service.Create(context.Background(), "user", "org", "workspace", definition)
	if err != nil {
		t.Fatal(err)
	}
	if created.Runtime != agentmodel.RuntimeClaudeCode || created.Provider != "anthropic" || created.Model != "gateway/claude-sonnet" {
		t.Fatalf("runtime profile settings were not applied: %#v", created.AgentDefinition)
	}
}
