package taskintegration

import "context"

// SourceIntegrationProvider validates a source and discovers its current tasks.
type SourceIntegrationProvider interface {
	Provider() TaskIntegrationSourceProvider
	Validate(context.Context, TaskIntegrationConfig, string) error
	Discover(context.Context, TaskIntegrationConfig, string, []string) ([]DiscoveredTask, error)
}
