package coderepository

import (
	"context"
	"encoding/json"
	"fmt"
)

type ProviderError struct {
	Code   string
	Status int
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("coding provider request failed (%s)", e.Code)
}

type Credentials struct {
	Token          string
	ProviderConfig json.RawMessage
}

type RepositoryPage struct {
	Repositories []RepositoryMetadata `json:"repositories"`
	NextPage     int                  `json:"nextPage,omitempty"`
}

type RepositoryMetadata struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	CloneURL      string `json:"cloneUrl"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"defaultBranch"`
}

// CodingProvider contains provider-specific validation and repository
// operations. Credential values are always explicit operation inputs.
type CodingProvider interface {
	ProviderName() string
	ConfigSchema() json.RawMessage
	ValidateConfig(json.RawMessage) error
	ValidateAuthentication(context.Context, json.RawMessage, Credentials) error
	DiscoverRepositories(context.Context, json.RawMessage, Credentials, int, int) (RepositoryPage, error)
	ResolveRepository(context.Context, json.RawMessage, Credentials, string) (RepositoryMetadata, error)
	NormalizeRepositoryURL(string) (string, error)
	CloneAuthenticated(context.Context, RepositoryMetadata, Credentials, string) error
	SyncAuthenticated(context.Context, RepositoryMetadata, Credentials, string) error
	PrepareAuthenticatedTaskCheckout(context.Context, RepositoryMetadata, Credentials, string, string) error
}

// LegacyRepositoryCheckout supports pre-integration public registrations while
// they remain in workspace data. New registrations always use CodingProvider.
type LegacyRepositoryCheckout interface {
	ClonePublic(context.Context, string, string) error
	SyncPublic(context.Context, string, string) error
	PreparePublicTaskCheckout(context.Context, string, string, string) error
}
