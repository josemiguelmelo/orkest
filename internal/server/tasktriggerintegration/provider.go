package tasktriggerintegration

import (
	"context"
	"encoding/json"
)

type ProviderDescriptor struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	ConfigSchema json.RawMessage `json:"configSchema"`
	Events       []string        `json:"events"`
}

type WebhookRequest struct {
	Headers map[string]string
	Body    []byte
}

// TaskTriggerProvider authenticates the original bytes before normalizing events.
// Unsupported notifications return no events. Configuration and event identifiers
// are provider-owned, so adding providers does not require a domain enum change.
type TaskTriggerProvider interface {
	Descriptor() ProviderDescriptor
	ValidateConfig(json.RawMessage) error
	VerifyAndDecode(context.Context, WebhookRequest, json.RawMessage, string) (string, []Event, error)
	ValidateResource(json.RawMessage, ResourceLink) error
}
