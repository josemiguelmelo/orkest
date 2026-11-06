package model

import (
	"encoding/json"
	"time"
)

type TaskIntegrationSourceProvider string

type TaskIntegration struct {
	ID          string                        `json:"id"`
	WorkspaceID string                        `json:"workspaceId"`
	WorkflowID  string                        `json:"workflowId"`
	Provider    TaskIntegrationSourceProvider `json:"provider"`
	Name        string                        `json:"name"`
	Enabled     bool                          `json:"enabled"`
	Config      json.RawMessage               `json:"config"`
	Labels      []string                      `json:"labels"`
	Interval    string                        `json:"interval"`
	LastSynced  *time.Time                    `json:"lastSynced,omitempty"`
	LastError   string                        `json:"lastError,omitempty"`
	CreatedAt   time.Time                     `json:"createdAt"`
	UpdatedAt   time.Time                     `json:"updatedAt"`
}

type TaskIntegrationInput struct {
	WorkflowID string                        `json:"workflowId"`
	Provider   TaskIntegrationSourceProvider `json:"provider"`
	Name       string                        `json:"name"`
	Enabled    bool                          `json:"enabled"`
	Config     json.RawMessage               `json:"config"`
	Labels     []string                      `json:"labels"`
	Interval   string                        `json:"interval,omitempty"`
	Credential string                        `json:"credential,omitempty"`
}

type SourceIntegrationProviderInfo struct {
	Provider     TaskIntegrationSourceProvider `json:"provider"`
	ConfigSchema json.RawMessage               `json:"configSchema"`
}

type DiscoveredTask struct {
	ID          string
	URL         string
	Title       string
	Description string
	Labels      []string
	Closed      bool
}
