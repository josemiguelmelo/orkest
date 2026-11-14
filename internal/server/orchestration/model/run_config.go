package model

import (
	"slices"

	"github.com/josemiguelmelo/orkest/internal/contracts/configvalidation"
	workflowmodel "github.com/josemiguelmelo/orkest/internal/server/workflow/model"
	mountmodel "github.com/josemiguelmelo/orkest/internal/server/workspacemount/model"
)

// RunConfig is the resolved configuration bound to one task execution run.
// It is separate from reusable agent definitions and process startup options.
type RunConfig struct {
	IntegrationGuard      *workflowmodel.IntegrationEvent `json:"-"`
	IntegrationWorkflowID string                          `json:"-"`
	IntegrationActorID    string                          `json:"-"`
	DeliveryToken         string                          `json:"-"`
	AgentDefinitionID     string                          `json:"agentDefinitionId,omitempty"`
	ExpectedAssigneeID    string                          `json:"-"`
	ExpectedSessionID     string                          `json:"-"`
	GuardAssignment       bool                            `json:"-"`
	ArtifactRevisionIDs   []string                        `json:"-"`
	RuntimeConfig
	// Platform-only image selection is persisted with the session, excluded by ForRuntime.
	ExecutionProfileID   string             `json:"executionProfileId,omitempty"`
	ExecutionImage       string             `json:"executionImage,omitempty"`
	RestoreSession       bool               `json:"restoreSession,omitempty"`
	EncryptedEnvironment string             `json:"encryptedEnvironment,omitempty"`
	HostMounts           []mountmodel.Mount `json:"hostMounts,omitempty"`
	// Repositories are executor-only checkout inputs and are intentionally
	// excluded from the agentctl HTTP configuration payload.
	Repositories     []WorkspaceRepository `json:"-"`
	ReservedRun      bool                  `json:"-"`
	InitialCommandID string                `json:"-"`
	InitialPrompt    string                `json:"-"`
	TaskContext      *TaskContextBaseline  `json:"taskContext,omitempty"`
}

// TaskContextBaseline records which task context was included in the session prompt.
// It is platform-only metadata and is excluded from RuntimeConfig/ForRuntime.
type TaskContextBaseline struct {
	Version      int               `json:"version"`
	Fingerprints map[string]string `json:"fingerprints"`
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
	c.ArtifactRevisionIDs = slices.Clone(c.ArtifactRevisionIDs)
	c.Repositories = slices.Clone(c.Repositories)
	c.HostMounts = slices.Clone(c.HostMounts)
	if c.TaskContext != nil {
		baseline := *c.TaskContext
		baseline.Fingerprints = make(map[string]string, len(c.TaskContext.Fingerprints))
		for key, value := range c.TaskContext.Fingerprints {
			baseline.Fingerprints[key] = value
		}
		c.TaskContext = &baseline
	}
	return c
}

// ForRuntime returns an independent snapshot without executor-only inputs.
func (c RunConfig) ForRuntime() RuntimeConfig { return c.RuntimeConfig.Clone() }
