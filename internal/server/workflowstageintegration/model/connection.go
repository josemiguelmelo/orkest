package model

import (
	"encoding/json"
	"time"
)

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
