package model

import (
	"slices"

	"github.com/josemiguelmelo/orkest/internal/contracts/configvalidation"
)

// RunConfig is the resolved configuration bound to one task execution run.
// It is separate from reusable agent definitions and process startup options.
type RunConfig struct {
	ArtifactRevisionIDs []string `json:"-"`
	RuntimeConfig
	// Platform-only image selection is persisted with the session, excluded by ForRuntime.
	ExecutionProfileID   string `json:"executionProfileId,omitempty"`
	ExecutionImage       string `json:"executionImage,omitempty"`
	RestoreSession       bool   `json:"restoreSession,omitempty"`
	EncryptedEnvironment string `json:"encryptedEnvironment,omitempty"`
	// Repositories are executor-only checkout inputs and are intentionally
	// excluded from the agentctl HTTP configuration payload.
	Repositories     []WorkspaceRepository `json:"-"`
	ReservedRun      bool                  `json:"-"`
	InitialCommandID string                `json:"-"`
	InitialPrompt    string                `json:"-"`
}

// WorkspaceRepository identifies a registered workspace repository for the
// executor to prepare in the task's durable working directory.
type WorkspaceRepository struct {
	ID                   string `json:"-"`
	ProviderName         string `json:"-"`
	URL                  string `json:"-"`
	IntegrationID        string `json:"-"`
	ProviderRepositoryID string `json:"-"`
	DefaultBranch        string `json:"-"`
	Name                 string `json:"-"`
}

// Validate checks the resolved run configuration without starting execution.
func (c RunConfig) Validate() error {
	if err := c.RuntimeConfig.Validate(); err != nil {
		return err
	}
	seen := make(map[string]bool, len(c.Repositories))
	for _, repository := range c.Repositories {
		issue := configvalidation.Repository(repository.ID, repository.ProviderName, repository.URL, repository.IntegrationID, repository.ProviderRepositoryID, repository.DefaultBranch, seen[repository.ID])
		if issue != nil {
			return validationError(issue)
		}
		seen[repository.ID] = true
	}
	return nil
}

// Clone gives the recipient ownership of all mutable configuration values.
func (c RunConfig) Clone() RunConfig {
	c.RuntimeConfig = c.RuntimeConfig.Clone()
	c.Repositories = slices.Clone(c.Repositories)
	return c
}

// ForRuntime returns an independent snapshot without executor-only inputs.
func (c RunConfig) ForRuntime() RuntimeConfig { return c.RuntimeConfig.Clone() }
