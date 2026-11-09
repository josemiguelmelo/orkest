package tasktriggerintegration

import (
	"context"

	"github.com/josemiguelmelo/orkest/internal/server/task"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
)

type ProcessingSnapshot struct {
	TaskFound    bool
	LinkActive   bool
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
	RuleWorkflows(context.Context, string, []Rule) ([]workflowmodel.Definition, error)
	Save(context.Context, ConfigurationPlan) (Integration, error)
	Delete(context.Context, string, string) error
	ResourceLinkSnapshot(context.Context, string, ResourceLink) (ResourceLinkSnapshot, error)
	CommitResourceLink(context.Context, ResourceLinkSnapshot, ResourceLinkChange) error
	Links(context.Context, string) ([]ResourceLink, error)
	IngestionSnapshot(context.Context, string, Event) (IngestionSnapshot, error)
	CommitIngestion(context.Context, IngestionSnapshot, string, IngestionDecision) error
	Deliveries(context.Context, string) ([]Delivery, error)
	NextProcessing(context.Context) (ProcessingSnapshot, bool, error)
	CommitProcessing(context.Context, ProcessingSnapshot, ProcessingResult) (bool, error)
}
