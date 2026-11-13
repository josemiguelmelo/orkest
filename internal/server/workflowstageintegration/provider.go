// Package workflowstageintegration administers connections and ingests external workflow events.
package workflowstageintegration

import (
	"context"
	"encoding/json"
	"time"

	stageintegrationmodel "github.com/josemiguelmelo/orkest/internal/server/workflowstageintegration/model"
)

// WorkflowStageIntegrationProvider owns provider configuration, webhook
// authentication, event mapping and action-specific parameter validation. No HTTP handler knows
// provider header names, signature algorithms or payload fields.
type WorkflowStageIntegrationProvider interface {
	Provider() string
	ConfigSchema() json.RawMessage
	ValidateConfig(json.RawMessage) error
	Actions() []stageintegrationmodel.ActionDefinition
	ValidateParameters(string, json.RawMessage) error
	HandleCallback(context.Context, stageintegrationmodel.CallbackInput) (stageintegrationmodel.CallbackResult, error)
	Matches(string, json.RawMessage, stageintegrationmodel.ProviderEvent) (bool, error)
}

// SubjectProvider performs provider I/O outside workflow database transactions.
type SubjectProvider interface {
	ResolveSubject(context.Context, json.RawMessage, string, string) (stageintegrationmodel.Subject, error)
	Reconcile(context.Context, json.RawMessage, string, stageintegrationmodel.Subject, time.Time) ([]stageintegrationmodel.ProviderEvent, error)
}

// SubjectMatcher evaluates provider-owned aggregate conditions.
type SubjectMatcher interface {
	MatchSubjects(string, json.RawMessage, stageintegrationmodel.ProviderEvent, []stageintegrationmodel.Subject) (stageintegrationmodel.SubjectMatch, error)
}
