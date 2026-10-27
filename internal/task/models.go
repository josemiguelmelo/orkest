package task

import "time"

type Task struct {
	SessionMode     string      `json:"-"`
	PromptText      string      `json:"-"`
	CommandID       string      `json:"-"`
	ID              string      `json:"id"`
	WorkspaceID     string      `json:"workspaceId"`
	Title           string      `json:"title"`
	Description     string      `json:"description"`
	SourceProvider  string      `json:"sourceProvider"`
	SourceID        string      `json:"sourceId,omitempty"`
	SourceURL       string      `json:"sourceUrl,omitempty"`
	Status          string      `json:"status"`
	AssigneeAgentID string      `json:"assigneeAgentId,omitempty"`
	Labels          []string    `json:"labels"`
	Components      []Component `json:"components"`
	CreatedAt       time.Time   `json:"createdAt"`
	UpdatedAt       time.Time   `json:"updatedAt"`
}

type Component struct {
	ID           string    `json:"id"`
	WorkspaceID  string    `json:"workspaceId"`
	Identifier   string    `json:"identifier"`
	Name         string    `json:"name"`
	RepositoryID string    `json:"repositoryId"`
	Description  string    `json:"description"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
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
	StartWork       bool     `json:"startWork,omitempty"`
	SessionMode     string   `json:"sessionMode,omitempty"`
	PromptText      string   `json:"promptText,omitempty"`
	CommandID       string   `json:"commandId,omitempty"`
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	Status          string   `json:"status"`
	AssigneeAgentID string   `json:"assigneeAgentId"`
	Labels          []string `json:"labels"`
	ComponentIDs    []string `json:"componentIds"`
}

type ComponentInput struct {
	Identifier   string `json:"identifier"`
	Name         string `json:"name"`
	RepositoryID string `json:"repositoryId"`
	Description  string `json:"description"`
}
