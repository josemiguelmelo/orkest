package agent

import "context"

import "github.com/josemiguelmelo/orkest/internal/workspace"

type Repository interface {
	Agents(context.Context, string, string) ([]Agent, error)
	Agent(context.Context, string, string, string) (Agent, error)
	CreateAgent(context.Context, string, string, AgentDefinition) (Agent, error)
	UpdateAgent(context.Context, string, string, string, AgentDefinition) (Agent, error)
	DeleteAgent(context.Context, string, string, string) error
}

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error)
}
