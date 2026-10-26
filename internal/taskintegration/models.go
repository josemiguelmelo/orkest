package taskintegration

import (
	"context"
	"time"
)

const (
	ProviderGitHub  = "github"
	ProviderJira    = "jira"
	DefaultInterval = 5 * time.Minute
)

type Source struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspaceId"`
	Provider    string     `json:"provider"`
	Name        string     `json:"name"`
	Enabled     bool       `json:"enabled"`
	Config      Config     `json:"config"`
	Labels      []string   `json:"labels"`
	Interval    string     `json:"interval"`
	LastSynced  *time.Time `json:"lastSynced,omitempty"`
	LastError   string     `json:"lastError,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// Config contains provider-specific non-secret settings. Credentials are always
// supplied separately and are never serialized from Source.
type Config struct {
	BaseURL    string   `json:"baseUrl,omitempty"`
	ProjectURL string   `json:"projectUrl,omitempty"`
	Projects   []string `json:"projects,omitempty"`
}

type SourceInput struct {
	Provider   string   `json:"provider"`
	Name       string   `json:"name"`
	Enabled    bool     `json:"enabled"`
	Config     Config   `json:"config"`
	Labels     []string `json:"labels"`
	Interval   string   `json:"interval,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

type RemoteTask struct {
	ID          string
	URL         string
	Title       string
	Description string
	Labels      []string
	Closed      bool
}

type TaskImporter interface {
	UpsertImportedTask(context.Context, string, string, string, RemoteTask) error
	RemoveMissingImportedTasks(context.Context, string, string, []string) error
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

type SourceIntegration interface {
	Name() string
	Validate(context.Context, Config, string) error
	Discover(context.Context, Config, string, []string) ([]RemoteTask, error)
}

type Repository interface {
	Create(context.Context, Source, string) (Source, error)
	Update(context.Context, string, string, Source, string) (Source, error)
	Delete(context.Context, string, string) error
	Get(context.Context, string, string) (Source, string, error)
	List(context.Context, string) ([]Source, error)
	Due(context.Context, time.Time) ([]Source, error)
	RecordSync(context.Context, string, time.Time, string) error
}
