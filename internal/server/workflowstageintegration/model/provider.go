package model

import (
	"encoding/json"
	"time"
)

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

// SubjectMatch supports provider-owned aggregate conditions without provider
// action switches in the ingestion or workflow engine.
type SubjectMatch struct {
	Matched            bool
	OncePerObservation bool
}

// Reconciliation binds normalized events to the same verified subject snapshot.
type Reconciliation struct {
	Subject Subject
	Events  []ProviderEvent
}
