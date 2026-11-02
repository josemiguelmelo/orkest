package taskintegration

import (
	"context"
	"time"

	"github.com/josemiguelmelo/orkest/internal/task"
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
	UpsertImportedTask(context.Context, string, string, string, string, DiscoveredTask, func(task.ImportedTaskSnapshot) (task.ImportedTaskEffects, error)) error
	RemoveMissingImportedTasks(context.Context, string, string, []string) error
}

// ImportedTaskPolicy is supplied by the task service.
type ImportedTaskPolicy interface {
	PlanImportedTask(task.ImportedTaskSnapshot) (task.ImportedTaskEffects, error)
}
