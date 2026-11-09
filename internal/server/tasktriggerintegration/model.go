// Package tasktriggerintegration ingests external events without owning execution.
package tasktriggerintegration

import (
	"encoding/json"
	"time"
)

type Rule struct {
	ID            string `json:"id"`
	Event         string `json:"event"`
	WorkflowID    string `json:"workflowId"`
	SourceStageID string `json:"sourceStageId"`
	TargetStageID string `json:"targetStageId"`
	Action        string `json:"action"`
	PromptText    string `json:"promptText,omitempty"`
}

type Integration struct {
	DiscoveryDiagnostic string          `json:"discoveryDiagnostic,omitempty"`
	ID                  string          `json:"id"`
	WorkspaceID         string          `json:"workspaceId"`
	OrganizationID      string          `json:"-"`
	ConfiguredBy        string          `json:"-"`
	Name                string          `json:"name"`
	Provider            string          `json:"provider"`
	Config              json.RawMessage `json:"config"`
	Enabled             bool            `json:"enabled"`
	Version             int64           `json:"version"`
	Rules               []Rule          `json:"rules"`
	EncryptedSecret     string          `json:"-"`
}

type Input struct {
	Name          string          `json:"name"`
	Provider      string          `json:"provider"`
	Config        json.RawMessage `json:"config"`
	Enabled       bool            `json:"enabled"`
	Version       int64           `json:"version"`
	Rules         []Rule          `json:"rules"`
	WebhookSecret string          `json:"webhookSecret,omitempty"`
}

type ResourceLink struct {
	TaskID       string `json:"taskId"`
	RepositoryID string `json:"repositoryId"`
	ResourceID   string `json:"resourceId"`
}

// Event contains identities, never provider SDK types or untrusted comment bodies.
type Event struct {
	HeadRepositoryID string    `json:"headRepositoryId,omitempty"`
	HeadBranch       string    `json:"headBranch,omitempty"`
	HeadCommitID     string    `json:"headCommitId,omitempty"`
	ReferenceURL     string    `json:"referenceUrl"`
	Key              string    `json:"key"`
	Type             string    `json:"type"`
	RepositoryID     string    `json:"repositoryId"`
	ResourceID       string    `json:"resourceId"`
	OccurredAt       time.Time `json:"occurredAt"`
}

type Delivery struct {
	Diagnostic    string    `json:"diagnostic,omitempty"`
	ID            string    `json:"id"`
	IntegrationID string    `json:"integrationId"`
	Event         Event     `json:"event"`
	Status        string    `json:"status"`
	ReceivedAt    time.Time `json:"receivedAt"`
}
