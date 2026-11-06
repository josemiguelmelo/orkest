package model

type TaskInput struct {
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

// StartCommand is an explicit execution request, separate from the task entity.
type StartCommand struct {
	Task                               Task
	CommandID, SessionMode, PromptText string
	DeliveryToken                      string
}
