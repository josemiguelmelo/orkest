package coderepository

import (
	"context"
	"encoding/json"

	coderepositorymodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
)

// CodingProvider contains provider-specific validation and repository
// operations. Credential values are always explicit operation inputs.
type CodingProvider interface {
	ProviderName() string
	ConfigSchema() json.RawMessage
	BaseURL(json.RawMessage) (string, error)
	ValidateConfig(json.RawMessage) error
	ValidateAuthentication(context.Context, json.RawMessage, coderepositorymodel.Credentials) error
	DiscoverRepositories(context.Context, json.RawMessage, coderepositorymodel.Credentials, int, int) (coderepositorymodel.RepositoryPage, error)
	ResolveRepository(context.Context, json.RawMessage, coderepositorymodel.Credentials, string) (coderepositorymodel.RepositoryMetadata, error)
	NormalizeRepositoryURL(string) (string, error)
	CloneAuthenticated(context.Context, coderepositorymodel.RepositoryMetadata, coderepositorymodel.Credentials, string) error
	SyncAuthenticated(context.Context, coderepositorymodel.RepositoryMetadata, coderepositorymodel.Credentials, string) error
	PrepareAuthenticatedTaskCheckout(context.Context, coderepositorymodel.RepositoryMetadata, coderepositorymodel.Credentials, string, string) error
}

// LegacyRepositoryCheckout supports pre-integration public registrations while
// they remain in workspace data. New registrations always use CodingProvider.
type LegacyRepositoryCheckout interface {
	ClonePublic(context.Context, string, string) error
	SyncPublic(context.Context, string, string) error
	PreparePublicTaskCheckout(context.Context, string, string, string) error
}
