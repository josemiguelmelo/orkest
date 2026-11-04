package agentctlhttp

import (
	wire "github.com/josemiguelmelo/orkest/internal/contracts/agentctl/v1"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
)

func toWireAgentDefinition(value orchestration.AgentDefinition) wire.AgentDefinition {
	result := wire.AgentDefinition{}
	result.Name = value.Name
	result.Description = value.Description
	result.Instructions = value.Instructions
	result.PermissionPolicy = value.PermissionPolicy
	return result
}

func toWireModelSelection(value orchestration.ModelSelection) wire.ModelSelection {
	result := wire.ModelSelection{}
	result.Provider = value.Provider
	result.ID = value.ID
	return result
}

func toWireMCPServer(value orchestration.MCPServer) wire.MCPServer {
	result := wire.MCPServer{}
	result.ID = value.ID
	result.Name = value.Name
	result.URL = value.URL
	result.Headers = value.Headers
	return result
}

func toWireSkill(value orchestration.Skill) wire.Skill {
	result := wire.Skill{}
	result.ID = value.ID
	result.SkillMD = value.SkillMD
	return result
}

func toWireGitCredential(value orchestration.GitCredential) wire.GitCredential {
	result := wire.GitCredential{}
	result.ProviderName = value.ProviderName
	result.ProviderConfig = value.ProviderConfig
	result.RepositoryURL = value.RepositoryURL
	result.RepositoryPath = value.RepositoryPath
	result.Token = value.Token
	return result
}

func toWireRuntimeConfig(value orchestration.RuntimeConfig) wire.RuntimeConfig {
	result := wire.RuntimeConfig{}
	result.OrganizationID = value.OrganizationID
	result.WorkspaceID = value.WorkspaceID
	result.TaskID = value.TaskID
	result.TaskSessionID = value.TaskSessionID
	result.RunID = value.RunID
	result.Runtime = string(value.Runtime)
	result.Agent = toWireAgentDefinition(value.Agent)
	result.Model = toWireModelSelection(value.Model)
	if value.MCPServers != nil {
		result.MCPServers = make([]wire.MCPServer, len(value.MCPServers))
		for i := range value.MCPServers {
			result.MCPServers[i] = toWireMCPServer(value.MCPServers[i])
		}
	}
	if value.Skills != nil {
		result.Skills = make([]wire.Skill, len(value.Skills))
		for i := range value.Skills {
			result.Skills[i] = toWireSkill(value.Skills[i])
		}
	}
	result.Environment = value.Environment
	if value.GitCredentials != nil {
		result.GitCredentials = make([]wire.GitCredential, len(value.GitCredentials))
		for i := range value.GitCredentials {
			result.GitCredentials[i] = toWireGitCredential(value.GitCredentials[i])
		}
	}
	result.GitCredentialsRequired = value.GitCredentialsRequired
	return result
}

func toWirePermissionResponse(value orchestration.PermissionResponse) wire.PermissionResponse {
	result := wire.PermissionResponse{}
	result.TaskID = value.TaskID
	result.RunID = value.RunID
	result.SessionID = value.SessionID
	result.RequestID = value.RequestID
	result.OptionID = value.OptionID
	return result
}

func toWireRunConfig(value orchestration.RunConfig) wire.RunConfig {
	result := wire.RunConfig{}
	result.RuntimeConfig = toWireRuntimeConfig(value.RuntimeConfig)
	result.ExecutionProfileID = value.ExecutionProfileID
	result.ExecutionImage = value.ExecutionImage
	result.RestoreSession = value.RestoreSession
	result.EncryptedEnvironment = value.EncryptedEnvironment
	return result
}

func fromWireUpdate(value wire.Update) orchestration.Update {
	result := orchestration.Update{}
	result.Kind = value.Kind
	result.Text = value.Text
	result.ToolCallID = value.ToolCallID
	result.Title = value.Title
	result.Status = value.Status
	result.ToolKind = value.ToolKind
	result.Input = value.Input
	result.Output = value.Output
	result.Truncated = value.Truncated
	if value.Locations != nil {
		result.Locations = make([]orchestration.ToolLocation, len(value.Locations))
		for i := range value.Locations {
			result.Locations[i] = fromWireToolLocation(value.Locations[i])
		}
	}
	if value.Content != nil {
		result.Content = make([]orchestration.ToolContent, len(value.Content))
		for i := range value.Content {
			result.Content[i] = fromWireToolContent(value.Content[i])
		}
	}
	return result
}

func fromWireToolLocation(value wire.ToolLocation) orchestration.ToolLocation {
	result := orchestration.ToolLocation{}
	result.Path = value.Path
	result.Line = value.Line
	return result
}

func fromWireToolContent(value wire.ToolContent) orchestration.ToolContent {
	result := orchestration.ToolContent{}
	result.Kind = value.Kind
	result.Path = value.Path
	result.OldText = value.OldText
	result.NewText = value.NewText
	result.Text = value.Text
	result.Truncated = value.Truncated
	return result
}

func fromWireEvent(value wire.Event) orchestration.AgentActivityEvent {
	result := orchestration.AgentActivityEvent{}
	result.At = value.At
	result.Sequence = value.Sequence
	result.Type = orchestration.EventType(value.Type)
	result.OrganizationID = value.OrganizationID
	result.WorkspaceID = value.WorkspaceID
	result.TaskID = value.TaskID
	result.RunID = value.RunID
	result.TaskSessionID = value.TaskSessionID
	result.SessionID = value.SessionID
	result.CommandID = value.CommandID
	result.RequestID = value.RequestID
	result.Text = value.Text
	if value.Update != nil {
		mapped := fromWireUpdate(*value.Update)
		result.Update = &mapped
	}
	if value.Permission != nil {
		mapped := fromWirePermission(*value.Permission)
		result.Permission = &mapped
	}
	result.StopReason = value.StopReason
	result.Error = value.Error
	return result
}

func fromWirePermissionOption(value wire.PermissionOption) orchestration.PermissionOption {
	result := orchestration.PermissionOption{}
	result.ID = value.ID
	result.Name = value.Name
	result.Kind = value.Kind
	return result
}

func fromWirePermission(value wire.Permission) orchestration.Permission {
	result := orchestration.Permission{}
	result.ToolCallID = value.ToolCallID
	result.Title = value.Title
	if value.Options != nil {
		result.Options = make([]orchestration.PermissionOption, len(value.Options))
		for i := range value.Options {
			result.Options[i] = fromWirePermissionOption(value.Options[i])
		}
	}
	return result
}
