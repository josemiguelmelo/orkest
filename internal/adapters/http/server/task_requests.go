package httpapi

import (
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
	"github.com/josemiguelmelo/orkest/internal/server/workflowautomation"
)

type taskMutationRequest struct {
	StageVersion    int64    `json:"stageVersion,omitempty"`
	WorkflowID      string   `json:"workflowId"`
	CompleteStage   bool     `json:"completeStage,omitempty"`
	ActorID         string   `json:"-"`
	StartWork       bool     `json:"startWork,omitempty"`
	SessionMode     string   `json:"sessionMode,omitempty"`
	PromptText      string   `json:"promptText,omitempty"`
	CommandID       string   `json:"commandId,omitempty"`
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	Status          string   `json:"status"`
	AssigneeAgentID string   `json:"assigneeAgentId"`
	Labels          []string `json:"labels"`
	RepositoryIDs   []string `json:"repositoryIds"`
}

func (r taskMutationRequest) domain() taskmodel.TaskInput {
	return taskmodel.TaskInput{StageVersion: r.StageVersion, WorkflowID: r.WorkflowID, CompleteStage: r.CompleteStage, ActorID: r.ActorID, StartWork: r.StartWork, SessionMode: r.SessionMode, PromptText: r.PromptText, CommandID: r.CommandID, Title: r.Title, Description: r.Description, Status: r.Status, AssigneeAgentID: r.AssigneeAgentID, Labels: r.Labels, RepositoryIDs: r.RepositoryIDs}
}

type workflowTransitionRequest struct {
	StageID      string `json:"stageId"`
	StageVersion int64  `json:"stageVersion"`
	Action       string `json:"action"`
	CommandID    string `json:"commandId"`
	SessionMode  string `json:"sessionMode"`
	PromptText   string `json:"promptText"`
}

func (r workflowTransitionRequest) domain() workflowautomation.TransitionRequest {
	return workflowautomation.TransitionRequest{StageID: r.StageID, StageVersion: r.StageVersion, Action: r.Action, CommandID: r.CommandID, SessionMode: r.SessionMode, PromptText: r.PromptText}
}
