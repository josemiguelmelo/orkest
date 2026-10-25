package coderepository

type CodeRepository struct {
	ID           string `json:"id"`
	WorkspaceID  string `json:"workspaceId"`
	ProviderName string `json:"providerName"`
	URL          string `json:"url"`
}

type CreateRequest struct {
	ProviderName string `json:"providerName"`
	URL          string `json:"url"`
}
