package taskintegration

import (
	"context"
	"encoding/json"

	taskintegrationmodel "github.com/josemiguelmelo/orkest/internal/server/taskintegration/model"
)

// SourceIntegrationProvider owns a provider's configuration schema, validation,
// connectivity check, and task discovery.
type SourceIntegrationProvider interface {
	Provider() taskintegrationmodel.TaskIntegrationSourceProvider
	ConfigSchema() json.RawMessage
	ValidateConfig(json.RawMessage) error
	Validate(context.Context, json.RawMessage, string) error
	Discover(context.Context, json.RawMessage, string, []string) ([]taskintegrationmodel.DiscoveredTask, error)
}
