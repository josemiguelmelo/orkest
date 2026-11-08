package tasktriggerintegration

import (
	"context"

	"github.com/josemiguelmelo/orkest/internal/server/task"
)

type ProcessingSnapshot struct {
	ActorRole    string
	Integration  Integration
	Delivery     Delivery
	StageVersion int64
	Task         task.TaskWorkflowSnapshot
}

type ProcessingResult struct {
	Status  string
	Effects *task.TaskWorkflowEffects
}

type TaskTriggerIntegrationRepository interface {
	List(context.Context, string) ([]Integration, error)
	Get(context.Context, string) (Integration, error)
	Save(context.Context, Integration, bool) (Integration, error)
	Delete(context.Context, string, string) error
	Link(context.Context, Integration, ResourceLink) error
	Links(context.Context, string) ([]ResourceLink, error)
	Unlink(context.Context, string, ResourceLink) error
	Ingest(context.Context, Integration, string, []Event) error
	Deliveries(context.Context, string) ([]Delivery, error)
	ProcessNext(context.Context, func(context.Context, ProcessingSnapshot) (ProcessingResult, error)) (bool, error)
}
