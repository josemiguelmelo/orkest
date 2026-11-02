package task

import "time"

type Task struct {
	StageName        string          `json:"-"`
	StageDescription string          `json:"-"`
	Closed           bool            `json:"closed"`
	StageVersion     int64           `json:"stageVersion"`
	WorkflowID       string          `json:"workflowId"`
	StageWorkState   WorkState       `json:"stageWorkState"`
	SessionMode      string          `json:"-"`
	PromptText       string          `json:"-"`
	CommandID        string          `json:"-"`
	ID               string          `json:"id"`
	WorkspaceID      string          `json:"workspaceId"`
	Title            string          `json:"title"`
	Description      string          `json:"description"`
	SourceProvider   string          `json:"sourceProvider"`
	SourceID         string          `json:"sourceId,omitempty"`
	SourceURL        string          `json:"sourceUrl,omitempty"`
	Status           string          `json:"status"`
	AssigneeAgentID  string          `json:"assigneeAgentId,omitempty"`
	Labels           []string        `json:"labels"`
	Repositories     []RepositoryRef `json:"repositories"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}

type RepositoryRef struct {
	ID           string `json:"id"`
	ProviderName string `json:"providerName"`
	Name         string `json:"name"`
	URL          string `json:"url"`
}

type Comment struct {
	ID           string    `json:"id"`
	WorkspaceID  string    `json:"workspaceId"`
	TaskID       string    `json:"taskId"`
	AuthorUserID string    `json:"authorUserId"`
	Body         string    `json:"body"`
	CreatedAt    time.Time `json:"createdAt"`
}

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
