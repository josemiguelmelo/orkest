// Package workflowstageintegration ingests external events through provider adapters.
package workflowstageintegration

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	artifactmodel "github.com/josemiguelmelo/orkest/internal/server/artifact/model"
	codemodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

var ErrUnauthenticated = errors.New("invalid integration callback authentication")

const MaxCallbackBytes = 1 << 20

type ActionDefinition struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	ParametersSchema json.RawMessage `json:"parametersSchema"`
}

type ProviderInfo struct {
	Provider     string             `json:"provider"`
	ConfigSchema json.RawMessage    `json:"configSchema"`
	Actions      []ActionDefinition `json:"actions"`
}

type Connection struct {
	ID                  string          `json:"id"`
	WorkspaceID         string          `json:"workspaceId"`
	Provider            string          `json:"provider"`
	Name                string          `json:"name"`
	Enabled             bool            `json:"enabled"`
	CodingIntegrationID string          `json:"codingIntegrationId"`
	RepositoryIDs       []string        `json:"repositoryIds"`
	Config              json.RawMessage `json:"config"`
	Version             int64           `json:"version"`
	ConfiguredBy        string          `json:"configuredBy"`
	UpdatedAt           time.Time       `json:"updatedAt"`
}

type ConnectionInput struct {
	Provider            string          `json:"provider"`
	Name                string          `json:"name"`
	Enabled             bool            `json:"enabled"`
	CodingIntegrationID string          `json:"codingIntegrationId"`
	RepositoryIDs       []string        `json:"repositoryIds"`
	Config              json.RawMessage `json:"config"`
	WebhookSecret       string          `json:"webhookSecret,omitempty"`
	Version             int64           `json:"version"`
}

type StoredConnection struct {
	Connection      Connection
	EncryptedSecret string
}

type CallbackRequest struct {
	Method  string
	Headers map[string][]string
	Body    []byte
}

type CallbackInput struct {
	Request CallbackRequest
	Config  json.RawMessage
	Secret  string
}

type CallbackResult struct {
	DeliveryID string
	Events     []ProviderEvent
	Ignored    bool
}

type IngestionReceipt struct {
	Duplicate bool `json:"duplicate"`
	Ignored   bool `json:"ignored"`
}

// Subject is an externally verified code-review identity. RepositoryID is the
// provider's immutable base-repository ID; ID is its immutable review-request ID.
type Subject struct {
	Kind         string `json:"kind"`
	ID           string `json:"id"`
	RepositoryID string `json:"repositoryId"`
	URL          string `json:"url"`
	HeadSHA      string `json:"headSha"`
	Merged       bool   `json:"merged"`
}

type ProviderEvent struct {
	ID         string          `json:"id"`
	ActionID   string          `json:"actionId"`
	Subject    Subject         `json:"subject"`
	OccurredAt time.Time       `json:"occurredAt"`
	Facts      json.RawMessage `json:"facts"`
}

// WorkflowStageIntegration owns provider configuration, webhook authentication,
// event mapping and action-specific parameter validation. No HTTP handler knows
// provider header names, signature algorithms or payload fields.
type WorkflowStageIntegration interface {
	Provider() string
	ConfigSchema() json.RawMessage
	ValidateConfig(json.RawMessage) error
	Actions() []ActionDefinition
	ValidateParameters(string, json.RawMessage) error
	HandleCallback(context.Context, CallbackInput) (CallbackResult, error)
	Matches(string, json.RawMessage, ProviderEvent) (bool, error)
}

// SubjectProvider performs provider I/O outside workflow database transactions.
type SubjectProvider interface {
	ResolveSubject(context.Context, json.RawMessage, string, string) (Subject, error)
	Reconcile(context.Context, json.RawMessage, string, Subject, time.Time) ([]ProviderEvent, error)
}

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error)
}

type CodingAccess interface {
	CodingCredential(context.Context, string, string) (codemodel.Credentials, error)
	ListIntegrations(context.Context, string, string, string) ([]codemodel.CodingIntegration, error)
}

// Candidate is the current task/stage/rule snapshot, selected by persistence.
// ObservationID deterministically includes connection and workflow versions,
// task stage-entry version and rule ID. Artifacts are current named input
// revisions from the latest producing stage entry, until first observation pin.
type Candidate struct {
	ObservationID                                                string
	Connection                                                   Connection
	OrganizationID, TaskID, WorkflowID, StageID, RuleID, ActorID string
	StageVersion, ConfigurationVersion                           int64
	ActionID                                                     string
	Parameters                                                   json.RawMessage
	Artifacts                                                    []artifactmodel.Revision
	Repositories                                                 []codemodel.CodeRepository
	Observation                                                  *Observation
}

type Observation struct {
	ID           string    `json:"id"`
	TaskID       string    `json:"taskId"`
	RuleID       string    `json:"ruleId"`
	StageVersion int64     `json:"stageVersion"`
	ActivatedAt  time.Time `json:"activatedAt"`
	ReconciledAt time.Time `json:"reconciledAt"`
	AttemptedAt  time.Time `json:"attemptedAt"`
	RevisionIDs  []string  `json:"revisionIds"`
	Subjects     []Subject `json:"subjects"`
	LastError    string    `json:"lastError"`
}

type Status struct {
	ID           string    `json:"id"`
	TaskID       string    `json:"taskId"`
	RuleID       string    `json:"ruleId"`
	StageVersion int64     `json:"stageVersion"`
	ActionID     string    `json:"actionId"`
	State        string    `json:"state"`
	Error        string    `json:"error"`
	CreatedAt    time.Time `json:"createdAt"`
}

// Repository transactions revalidate all supplied candidate/connection versions
// before admitting observations or events. External provider I/O never occurs
// while these transactions are open.
type IntegrationRepository interface {
	ListConnections(context.Context, string) ([]Connection, error)
	GetConnection(context.Context, string, string) (StoredConnection, error)
	CallbackConnection(context.Context, string) (StoredConnection, error)
	CreateConnection(context.Context, StoredConnection) (Connection, error)
	UpdateConnection(context.Context, StoredConnection) (Connection, error)
	DeleteConnection(context.Context, string, string) error
	RecordReceipt(context.Context, Connection, CallbackResult) (bool, error)
	Candidates(context.Context) ([]Candidate, error)
	SaveObservation(context.Context, Candidate, Observation) error
	RecordEvents(context.Context, Connection, []ProviderEvent) error
	Events(context.Context, Candidate, Observation) ([]ProviderEvent, error)
	AdmitEvent(context.Context, Candidate, Observation, ProviderEvent, string) error
	ListStatus(context.Context, string, string) ([]Status, error)
	Retry(context.Context, string, string) error
}

// SubjectMatch supports provider-owned aggregate conditions without provider
// action switches in the ingestion or workflow engine.
type SubjectMatch struct {
	Matched            bool
	OncePerObservation bool
}

type SubjectMatcher interface {
	MatchSubjects(string, json.RawMessage, ProviderEvent, []Subject) (SubjectMatch, error)
}
