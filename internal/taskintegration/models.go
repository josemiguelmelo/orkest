package taskintegration

import (
	"encoding/json"
	"time"
)

type TaskIntegrationSourceProvider string

const (
	DefaultInterval = 5 * time.Minute
)

type TaskIntegration struct {
	ID          string                        `json:"id"`
	WorkspaceID string                        `json:"workspaceId"`
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
