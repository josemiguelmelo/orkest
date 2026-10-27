package coderepository

import (
	"context"
	"encoding/json"
)

type CodeRepository struct {
	ID                   string `json:"id"`
	WorkspaceID          string `json:"workspaceId"`
	ProviderName         string `json:"providerName"`
	URL                  string `json:"url"`
	IntegrationID        string `json:"integrationId,omitempty"`
	ProviderRepositoryID string `json:"providerRepositoryId,omitempty"`
	DefaultBranch        string `json:"defaultBranch,omitempty"`
	Name                 string `json:"name,omitempty"`
	Private              bool   `json:"private"`
}

type CreateRequest struct {
	IntegrationID        string `json:"integrationId"`
	ProviderRepositoryID string `json:"providerRepositoryId"`
}

type IntegratedCodeRepositoryRepository interface {
	CreateIntegratedCodeRepository(context.Context, CodeRepository) (CodeRepository, error)
}

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

type CodingIntegrationRepository interface {
	ListCodingIntegrations(context.Context, string) ([]CodingIntegration, error)
	GetCodingIntegration(context.Context, string, string) (CodingIntegration, string, error)
	CreateCodingIntegration(context.Context, CodingIntegration, string) (CodingIntegration, error)
	UpdateCodingIntegration(context.Context, CodingIntegration, string) error
	DeleteCodingIntegration(context.Context, string, string) error
}
