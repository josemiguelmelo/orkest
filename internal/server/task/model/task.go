package model

import (
	"time"
)

type Task struct {
	AutomationPending bool            `json:"automationPending"`
	StageName         string          `json:"-"`
	StageDescription  string          `json:"-"`
	Closed            bool            `json:"closed"`
	StageVersion      int64           `json:"stageVersion"`
	WorkflowID        string          `json:"workflowId"`
	StageWorkState    WorkState       `json:"stageWorkState"`
	ID                string          `json:"id"`
	WorkspaceID       string          `json:"workspaceId"`
	Title             string          `json:"title"`
	Description       string          `json:"description"`
	SourceProvider    string          `json:"sourceProvider"`
	SourceID          string          `json:"sourceId,omitempty"`
	SourceURL         string          `json:"sourceUrl,omitempty"`
	Status            string          `json:"status"`
	AssigneeAgentID   string          `json:"assigneeAgentId,omitempty"`
	Labels            []string        `json:"labels"`
	Repositories      []RepositoryRef `json:"repositories"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
}

type RepositoryRef struct {
	ID           string `json:"id"`
	ProviderName string `json:"providerName"`
	Name         string `json:"name"`
	URL          string `json:"url"`
}

// WorkState describes the execution state of a task's current workflow stage.
type WorkState string

const (
	WorkStateIdle      WorkState = "idle"
	WorkStateQueued    WorkState = "queued"
	WorkStateRunning   WorkState = "running"
	WorkStateCompleted WorkState = "completed"
	WorkStateFailed    WorkState = "failed"
)
