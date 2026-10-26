package taskintegration

import (
	"time"
)

type TaskIntegrationSourceProvider string

const (
	TaskIntegrationSourceProviderGitHubCloud TaskIntegrationSourceProvider = "github_cloud"
	TaskIntegrationSourceProviderJiraServer  TaskIntegrationSourceProvider = "jira_server"
	DefaultInterval                                                        = 5 * time.Minute
)

type TaskIntegration struct {
	ID          string                        `json:"id"`
	WorkspaceID string                        `json:"workspaceId"`
	Provider    TaskIntegrationSourceProvider `json:"provider"`
	Name        string                        `json:"name"`
	Enabled     bool                          `json:"enabled"`
	Config      TaskIntegrationConfig         `json:"config"`
	Labels      []string                      `json:"labels"`
	Interval    string                        `json:"interval"`
	LastSynced  *time.Time                    `json:"lastSynced,omitempty"`
	LastError   string                        `json:"lastError,omitempty"`
	CreatedAt   time.Time                     `json:"createdAt"`
	UpdatedAt   time.Time                     `json:"updatedAt"`
}

// TaskIntegrationConfig contains provider-specific non-secret settings. Credentials are always
// supplied separately and are never serialized from TaskIntegration.
type TaskIntegrationConfig struct {
	BaseURL    string   `json:"baseUrl,omitempty"`
	ProjectURL string   `json:"projectUrl,omitempty"`
	Projects   []string `json:"projects,omitempty"`
}

type TaskIntegrationInput struct {
	Provider   TaskIntegrationSourceProvider `json:"provider"`
	Name       string                        `json:"name"`
	Enabled    bool                          `json:"enabled"`
	Config     TaskIntegrationConfig         `json:"config"`
	Labels     []string                      `json:"labels"`
	Interval   string                        `json:"interval,omitempty"`
	Credential string                        `json:"credential,omitempty"`
}

type DiscoveredTask struct {
	ID          string
	URL         string
	Title       string
	Description string
	Labels      []string
	Closed      bool
}

type ValidationError struct {
	Code  string
	Cause error
}

func (e *ValidationError) Error() string {
	if e.Code != "" {
		return e.Code
	}
	return "integration connectivity validation failed"
}
func (e *ValidationError) Unwrap() error { return e.Cause }

type ConfigurationError struct{ Code string }

func (e *ConfigurationError) Error() string { return e.Code }
