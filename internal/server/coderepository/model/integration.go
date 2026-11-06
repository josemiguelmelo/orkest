package model

import (
	"encoding/json"
)

type CodingIntegration struct {
	ID             string          `json:"id"`
	WorkspaceID    string          `json:"workspaceId"`
	Name           string          `json:"name"`
	ProviderName   string          `json:"providerName"`
	ProviderConfig json.RawMessage `json:"providerConfig"`
	UpdatedAt      string          `json:"updatedAt,omitempty"`
}

type CodingIntegrationInput struct {
	Name           string          `json:"name"`
	ProviderName   string          `json:"providerName"`
	ProviderConfig json.RawMessage `json:"providerConfig"`
	Credential     string          `json:"credential,omitempty"`
}
