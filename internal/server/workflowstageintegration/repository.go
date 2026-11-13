package workflowstageintegration

import (
	"context"
	"encoding/json"
	"time"

	artifactmodel "github.com/josemiguelmelo/orkest/internal/server/artifact/model"
	codemodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
	stageintegrationmodel "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration/model"
)

type StoredConnection struct {
	Connection      stageintegrationmodel.Connection
	EncryptedSecret string
}

// Candidate is the current task/stage/rule snapshot, selected by persistence.
// ObservationID deterministically includes connection and workflow versions,
// task stage-entry version and rule ID. Definition and AvailableArtifacts form a
// consistent snapshot; the service resolves inputs until first observation pin.
type Candidate struct {
	ObservationID                                                string
	Connection                                                   stageintegrationmodel.Connection
	OrganizationID, TaskID, WorkflowID, StageID, RuleID, ActorID string
	StageVersion, ConfigurationVersion                           int64
	ActionID                                                     string
	Parameters                                                   json.RawMessage
	Definition                                                   workflowmodel.Definition
	AvailableArtifacts                                           []artifactmodel.Revision
	Repositories                                                 []codemodel.CodeRepository
	Observation                                                  *Observation
}

type Observation struct {
	ID           string                          `json:"id"`
	TaskID       string                          `json:"taskId"`
	RuleID       string                          `json:"ruleId"`
	StageVersion int64                           `json:"stageVersion"`
	ActivatedAt  time.Time                       `json:"activatedAt"`
	ReconciledAt time.Time                       `json:"reconciledAt"`
	AttemptedAt  time.Time                       `json:"attemptedAt"`
	RevisionIDs  []string                        `json:"revisionIds"`
	Subjects     []stageintegrationmodel.Subject `json:"subjects"`
	LastError    string                          `json:"lastError"`
}

// WorkflowStageIntegrationRepository revalidates candidate and connection versions
// before admitting observations or events. External provider I/O never occurs
// while its transactions are open.
type WorkflowStageIntegrationRepository interface {
	ListConnections(context.Context, string) ([]stageintegrationmodel.Connection, error)
	GetConnection(context.Context, string, string) (StoredConnection, error)
	CallbackConnection(context.Context, string) (StoredConnection, error)
	CreateConnection(context.Context, StoredConnection) (stageintegrationmodel.Connection, error)
	UpdateConnection(context.Context, StoredConnection) (stageintegrationmodel.Connection, error)
	DeleteConnection(context.Context, string, string) error
	RecordReceipt(context.Context, stageintegrationmodel.Connection, stageintegrationmodel.CallbackResult) (bool, error)
	Candidates(context.Context) ([]Candidate, error)
	SaveObservation(context.Context, Candidate, Observation) error
	RecordEvents(context.Context, stageintegrationmodel.Connection, []stageintegrationmodel.ProviderEvent) error
	Events(context.Context, Candidate, Observation) ([]stageintegrationmodel.ProviderEvent, error)
	AdmitEvent(context.Context, Candidate, Observation, stageintegrationmodel.ProviderEvent, string) error
	ListStatus(context.Context, string, string) ([]stageintegrationmodel.Status, error)
	Retry(context.Context, string, string) error
}
