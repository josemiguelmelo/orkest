package agent

import (
	"context"

	agentmodel "github.com/josemiguelmelo/orkest/internal/server/agent/model"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type AgentDefinitionRepository interface {
	Agents(context.Context, string, string) ([]agentmodel.Agent, error)
	Agent(context.Context, string, string, string) (agentmodel.Agent, error)
	CreateAgent(context.Context, string, string, agentmodel.AgentDefinition) (agentmodel.Agent, error)
	UpdateAgent(context.Context, string, string, string, agentmodel.AgentDefinition) (agentmodel.Agent, error)
	DeleteAgent(context.Context, string, string, string) error
}

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error)
}

type MCPServerAccess interface {
	Exists(context.Context, string, string, []string) (bool, error)
}

type SkillAccess interface {
	ValidateAgentSkills(context.Context, string, string, []string) error
}
