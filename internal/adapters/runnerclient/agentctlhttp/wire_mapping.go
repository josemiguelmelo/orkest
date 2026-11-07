package agentctlhttp

import (
	wire "github.com/josemiguelmelo/orkest/internal/contracts/agentctl/v1"
	"github.com/josemiguelmelo/orkest/internal/platform/sliceutil"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

func toWireAgentDefinition(value orchestrationmodel.AgentDefinition) wire.AgentDefinition {
	result := wire.AgentDefinition{}
	result.Name = value.Name
	result.Description = value.Description
	result.Instructions = value.Instructions
	result.PermissionPolicy = value.PermissionPolicy
	return result
}

func toWireModelSelection(value orchestrationmodel.ModelSelection) wire.ModelSelection {
	result := wire.ModelSelection{}
	result.Provider = value.Provider
	result.ID = value.ID
	return result
}

func toWireMCPServer(value orchestrationmodel.MCPServer) wire.MCPServer {
	result := wire.MCPServer{}
	result.ID = value.ID
	result.Name = value.Name
	result.URL = value.URL
	result.Headers = value.Headers
	return result
}

func toWireSkill(value orchestrationmodel.Skill) wire.Skill {
	result := wire.Skill{}
	result.ID = value.ID
	result.SkillMD = value.SkillMD
	return result
}

func toWireGitCredential(value orchestrationmodel.GitCredential) wire.GitCredential {
	result := wire.GitCredential{}
	result.ProviderName = value.ProviderName
	result.ProviderConfig = value.ProviderConfig
	result.RepositoryURL = value.RepositoryURL
	result.RepositoryPath = value.RepositoryPath
	result.Token = value.Token
	return result
}

func toWireRuntimeConfig(value orchestrationmodel.RuntimeConfig) wire.RuntimeConfig {
	result := wire.RuntimeConfig{}
	result.OrganizationID = value.OrganizationID
	result.WorkspaceID = value.WorkspaceID
	result.TaskID = value.TaskID
	result.TaskSessionID = value.TaskSessionID
	result.RunID = value.RunID
	result.Runtime = string(value.Runtime)
	result.Agent = toWireAgentDefinition(value.Agent)
	result.Model = toWireModelSelection(value.Model)
	result.MCPServers = sliceutil.Map(value.MCPServers, toWireMCPServer)
	result.Skills = sliceutil.Map(value.Skills, toWireSkill)
	result.Environment = value.Environment
	result.GitCredentials = sliceutil.Map(value.GitCredentials, toWireGitCredential)
	result.GitCredentialsRequired = value.GitCredentialsRequired
	return result
}

func toWirePermissionResponse(value orchestrationmodel.PermissionResponse) wire.PermissionResponse {
	result := wire.PermissionResponse{}
	result.TaskID = value.TaskID
	result.RunID = value.RunID
	result.SessionID = value.SessionID
	result.RequestID = value.RequestID
	result.OptionID = value.OptionID
	return result
}

func toWireRunConfig(value orchestrationmodel.RunConfig) wire.RunConfig {
	result := wire.RunConfig{}
	result.RuntimeConfig = toWireRuntimeConfig(value.RuntimeConfig)
	result.ExecutionProfileID = value.ExecutionProfileID
	result.ExecutionImage = value.ExecutionImage
	result.RestoreSession = value.RestoreSession
	result.EncryptedEnvironment = value.EncryptedEnvironment
	return result
}

func fromWireUpdate(value wire.Update) orchestrationmodel.Update {
	result := orchestrationmodel.Update{}
	result.Kind = value.Kind
	result.Text = value.Text
	result.ToolCallID = value.ToolCallID
	result.Title = value.Title
	result.Status = value.Status
	result.ToolKind = value.ToolKind
	result.Input = value.Input
	result.Output = value.Output
	result.Truncated = value.Truncated
	result.Locations = sliceutil.Map(value.Locations, fromWireToolLocation)
	result.Content = sliceutil.Map(value.Content, fromWireToolContent)
	return result
}

func fromWireToolLocation(value wire.ToolLocation) orchestrationmodel.ToolLocation {
	result := orchestrationmodel.ToolLocation{}
	result.Path = value.Path
	result.Line = value.Line
	return result
}

func fromWireToolContent(value wire.ToolContent) orchestrationmodel.ToolContent {
	result := orchestrationmodel.ToolContent{}
	result.Kind = value.Kind
	result.Path = value.Path
	result.OldText = value.OldText
	result.NewText = value.NewText
	result.Text = value.Text
	result.Truncated = value.Truncated
	return result
}

func fromWireEvent(value wire.Event) orchestrationmodel.AgentActivityEvent {
	result := orchestrationmodel.AgentActivityEvent{}
	result.At = value.At
	result.Sequence = value.Sequence
	result.Type = orchestrationmodel.EventType(value.Type)
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
	if value.Question != nil {
		mapped := fromWireQuestion(*value.Question)
		result.Question = &mapped
	}
	if value.QuestionResponse != nil {
		mapped := fromWireQuestionResponse(*value.QuestionResponse)
		result.QuestionResponse = &mapped
	}
	result.StopReason = value.StopReason
	result.Error = value.Error
	return result
}

func fromWirePermissionOption(value wire.PermissionOption) orchestrationmodel.PermissionOption {
	result := orchestrationmodel.PermissionOption{}
	result.ID = value.ID
	result.Name = value.Name
	result.Kind = value.Kind
	return result
}

func fromWirePermission(value wire.Permission) orchestrationmodel.Permission {
	result := orchestrationmodel.Permission{}
	result.ToolCallID = value.ToolCallID
	result.Title = value.Title
	result.Options = sliceutil.Map(value.Options, fromWirePermissionOption)
	return result
}

func fromWireQuestion(value wire.Question) orchestrationmodel.Question {
	return orchestrationmodel.Question{ToolCallID: value.ToolCallID, Message: value.Message, Schema: value.Schema}
}

func fromWireQuestionAnswer(value wire.QuestionAnswer) orchestrationmodel.QuestionAnswer {
	return orchestrationmodel.QuestionAnswer{Action: value.Action, Content: value.Content}
}

func fromWireQuestionResponse(value wire.QuestionResponse) orchestrationmodel.QuestionResponse {
	return orchestrationmodel.QuestionResponse{AnsweredBy: value.AnsweredBy, TaskID: value.TaskID, TaskSessionID: value.TaskSessionID, RunID: value.RunID, SessionID: value.SessionID, CommandID: value.CommandID, RequestID: value.RequestID, SubmissionID: value.SubmissionID, Answer: fromWireQuestionAnswer(value.Answer)}
}

func toWireQuestionAnswer(value orchestrationmodel.QuestionAnswer) wire.QuestionAnswer {
	return wire.QuestionAnswer{Action: value.Action, Content: value.Content}
}

func toWireQuestionResponse(value orchestrationmodel.QuestionResponse) wire.QuestionResponse {
	return wire.QuestionResponse{AnsweredBy: value.AnsweredBy, TaskID: value.TaskID, TaskSessionID: value.TaskSessionID, RunID: value.RunID, SessionID: value.SessionID, CommandID: value.CommandID, RequestID: value.RequestID, SubmissionID: value.SubmissionID, Answer: toWireQuestionAnswer(value.Answer)}
}
