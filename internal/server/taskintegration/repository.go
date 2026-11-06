package taskintegration

import (
	"context"
	"time"

	"github.com/josemiguelmelo/orkest/internal/server/task"
	taskintegrationmodel "github.com/josemiguelmelo/orkest/internal/server/taskintegration/model"
)

// TaskIntegrationRepository persists workspace-scoped provider configurations.
type TaskIntegrationRepository interface {
	Create(context.Context, taskintegrationmodel.TaskIntegration, string) (taskintegrationmodel.TaskIntegration, error)
	Update(context.Context, string, string, taskintegrationmodel.TaskIntegration, string) (taskintegrationmodel.TaskIntegration, error)
	Delete(context.Context, string, string) error
	Get(context.Context, string, string) (taskintegrationmodel.TaskIntegration, string, error)
	List(context.Context, string) ([]taskintegrationmodel.TaskIntegration, error)
	Due(context.Context, time.Time) ([]taskintegrationmodel.TaskIntegration, error)
	RecordSync(context.Context, string, time.Time, string) error
}

// ImportedTaskRepository owns idempotent persistence and source reconciliation.
type ImportedTaskRepository interface {
	UpsertImportedTask(context.Context, string, string, string, string, taskintegrationmodel.DiscoveredTask, func(task.ImportedTaskSnapshot) (task.ImportedTaskEffects, error)) error
	RemoveMissingImportedTasks(context.Context, string, string, []string) error
}

// ImportedTaskPolicy is supplied by the task service.
type ImportedTaskPolicy interface {
	PlanImportedTask(task.ImportedTaskSnapshot) (task.ImportedTaskEffects, error)
}
