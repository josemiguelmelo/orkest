// Package repositoryobservation defines credential-free checkout and Git evidence contracts.
package repositoryobservation

const ManifestName = ".orkest-repositories.json"

type Checkout struct {
	RepositoryID         string `json:"repositoryId"`
	Provider             string `json:"provider"`
	ProviderRepositoryID string `json:"providerRepositoryId"`
	Path                 string `json:"path"`
	DefaultBranch        string `json:"defaultBranch"`
	RepositoryURL        string `json:"repositoryUrl"`
}

type Branch struct {
	Name               string `json:"name"`
	CommitID           string `json:"commitId"`
	EligibleForLinking bool   `json:"eligibleForLinking,omitempty"`
}

// Snapshot includes all local refs, allowing deletion and restore reconciliation.
type Snapshot struct {
	Checkout   Checkout `json:"checkout"`
	Branches   []Branch `json:"branches"`
	Diagnostic string   `json:"diagnostic,omitempty"`
}
