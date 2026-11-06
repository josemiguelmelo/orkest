package model

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
