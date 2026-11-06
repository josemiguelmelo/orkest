package httpapi

import (
	configmodel "github.com/josemiguelmelo/orkest/internal/agentctl/config/model"
	"github.com/josemiguelmelo/orkest/internal/agentctl/interaction"
	wire "github.com/josemiguelmelo/orkest/internal/contracts/agentctl/v1"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/platform/sliceutil"
)

func toWireUpdate(value interaction.Update) wire.Update {
	result := wire.Update{}
	result.Kind = value.Kind
	result.Text = value.Text
	result.ToolCallID = value.ToolCallID
	result.Title = value.Title
	result.Status = value.Status
	result.ToolKind = value.ToolKind
	result.Input = value.Input
	result.Output = value.Output
	result.Truncated = value.Truncated
	result.Locations = sliceutil.Map(value.Locations, toWireToolLocation)
	result.Content = sliceutil.Map(value.Content, toWireToolContent)
	return result
}

func toWireToolLocation(value interaction.ToolLocation) wire.ToolLocation {
	result := wire.ToolLocation{}
	result.Path = value.Path
	result.Line = value.Line
	return result
}

func toWireToolContent(value interaction.ToolContent) wire.ToolContent {
	result := wire.ToolContent{}
	result.Kind = value.Kind
	result.Path = value.Path
	result.OldText = value.OldText
	result.NewText = value.NewText
	result.Text = value.Text
	result.Truncated = value.Truncated
	return result
}

func toWireEvent(value interaction.Event) wire.Event {
	result := wire.Event{}
	result.At = value.At
	result.Sequence = value.Sequence
	result.Type = string(value.Type)
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
		mapped := toWireUpdate(*value.Update)
		result.Update = &mapped
	}
	if value.Permission != nil {
		mapped := toWirePermission(*value.Permission)
		result.Permission = &mapped
	}
	result.StopReason = value.StopReason
	result.Error = value.Error
	return result
}

func toWirePermissionOption(value interaction.PermissionOption) wire.PermissionOption {
	result := wire.PermissionOption{}
	result.ID = value.ID
	result.Name = value.Name
	result.Kind = value.Kind
	return result
}

func toWirePermission(value interaction.Permission) wire.Permission {
	result := wire.Permission{}
	result.ToolCallID = value.ToolCallID
	result.Title = value.Title
	result.Options = sliceutil.Map(value.Options, toWirePermissionOption)
	return result
}

func fromWireAgentDefinition(value wire.AgentDefinition) configmodel.AgentDefinition {
	result := configmodel.AgentDefinition{}
	result.Name = value.Name
	result.Description = value.Description
	result.Instructions = value.Instructions
	result.PermissionPolicy = value.PermissionPolicy
	return result
}

func fromWireModelSelection(value wire.ModelSelection) configmodel.ModelSelection {
	result := configmodel.ModelSelection{}
	result.Provider = value.Provider
	result.ID = value.ID
	return result
}

func fromWireMCPServer(value wire.MCPServer) configmodel.MCPServer {
	result := configmodel.MCPServer{}
	result.ID = value.ID
	result.Name = value.Name
	result.URL = value.URL
	result.Headers = value.Headers
	return result
}

func fromWireSkill(value wire.Skill) configmodel.Skill {
	result := configmodel.Skill{}
	result.ID = value.ID
	result.SkillMD = value.SkillMD
	return result
}

func fromWireGitCredential(value wire.GitCredential) configmodel.GitCredential {
	result := configmodel.GitCredential{}
	result.ProviderName = value.ProviderName
	result.ProviderConfig = value.ProviderConfig
	result.RepositoryURL = value.RepositoryURL
	result.RepositoryPath = value.RepositoryPath
	result.Token = value.Token
	return result
}

func fromWireRuntimeConfig(value wire.RuntimeConfig) configmodel.RuntimeConfig {
	result := configmodel.RuntimeConfig{}
	result.OrganizationID = value.OrganizationID
	result.WorkspaceID = value.WorkspaceID
	result.TaskID = value.TaskID
	result.TaskSessionID = value.TaskSessionID
	result.RunID = value.RunID
	result.Runtime = coreruntime.Runtime(value.Runtime)
	result.Agent = fromWireAgentDefinition(value.Agent)
	result.Model = fromWireModelSelection(value.Model)
	result.MCPServers = sliceutil.Map(value.MCPServers, fromWireMCPServer)
	result.Skills = sliceutil.Map(value.Skills, fromWireSkill)
	result.Environment = value.Environment
	result.GitCredentials = sliceutil.Map(value.GitCredentials, fromWireGitCredential)
	result.GitCredentialsRequired = value.GitCredentialsRequired
	return result
}

func fromWirePermissionResponse(value wire.PermissionResponse) interaction.PermissionResponse {
	result := interaction.PermissionResponse{}
	result.TaskID = value.TaskID
	result.RunID = value.RunID
	result.SessionID = value.SessionID
	result.RequestID = value.RequestID
	result.OptionID = value.OptionID
	return result
}

func fromWireRunConfig(value wire.RunConfig) configmodel.RunConfig {
	result := configmodel.RunConfig{}
	result.RuntimeConfig = fromWireRuntimeConfig(value.RuntimeConfig)
	result.ExecutionProfileID = value.ExecutionProfileID
	result.ExecutionImage = value.ExecutionImage
	result.RestoreSession = value.RestoreSession
	result.EncryptedEnvironment = value.EncryptedEnvironment
	return result
}
