package taskintegration

import (
	"context"
	"encoding/json"
)

// SourceIntegrationProvider owns a provider's configuration schema, validation,
// connectivity check, and task discovery.
type SourceIntegrationProvider interface {
	Provider() TaskIntegrationSourceProvider
	ConfigSchema() json.RawMessage
	ValidateConfig(json.RawMessage) error
	Validate(context.Context, json.RawMessage, string) error
	Discover(context.Context, json.RawMessage, string, []string) ([]DiscoveredTask, error)
}
