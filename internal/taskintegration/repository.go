package taskintegration

import (
	"context"
	"time"
)

// TaskIntegrationRepository persists workspace-scoped provider configurations.
type TaskIntegrationRepository interface {
	Create(context.Context, TaskIntegration, string) (TaskIntegration, error)
	Update(context.Context, string, string, TaskIntegration, string) (TaskIntegration, error)
	Delete(context.Context, string, string) error
	Get(context.Context, string, string) (TaskIntegration, string, error)
	List(context.Context, string) ([]TaskIntegration, error)
	Due(context.Context, time.Time) ([]TaskIntegration, error)
	RecordSync(context.Context, string, time.Time, string) error
}

// ImportedTaskRepository owns idempotent persistence and source reconciliation.
type ImportedTaskRepository interface {
	UpsertImportedTask(context.Context, string, string, string, DiscoveredTask) error
	RemoveMissingImportedTasks(context.Context, string, string, []string) error
}
