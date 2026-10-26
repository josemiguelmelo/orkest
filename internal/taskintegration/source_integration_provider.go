package taskintegration

import "context"

// SourceIntegrationProvider parses provider project URLs, validates a source,
// and discovers its current tasks.
type SourceIntegrationProvider interface {
	Provider() TaskIntegrationSourceProvider
	ParseProjectURL(string) error
	Validate(context.Context, TaskIntegrationConfig, string) error
	Discover(context.Context, TaskIntegrationConfig, string, []string) ([]DiscoveredTask, error)
}
