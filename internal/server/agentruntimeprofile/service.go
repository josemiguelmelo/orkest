package agentruntimeprofile

import (
	"context"
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	agentmodel "github.com/josemiguelmelo/orkest/internal/server/agent/model"
	agentruntimeprofilemodel "github.com/josemiguelmelo/orkest/internal/server/agentruntimeprofile/model"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error)
}

type AgentRuntimeProfileService struct {
	repository AgentRuntimeProfileRepository
	workspaces WorkspaceAccess
}

func NewAgentRuntimeProfileService(repository AgentRuntimeProfileRepository, workspaces WorkspaceAccess) *AgentRuntimeProfileService {
	return &AgentRuntimeProfileService{repository: repository, workspaces: workspaces}
}

func (s *AgentRuntimeProfileService) List(ctx context.Context, userID, orgID, workspaceID string) ([]agentruntimeprofilemodel.Profile, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repository.List(ctx, workspaceID)
}

func (s *AgentRuntimeProfileService) Save(ctx context.Context, userID, orgID, workspaceID, id string, input agentruntimeprofilemodel.Profile) (agentruntimeprofilemodel.Profile, error) {
	w, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return agentruntimeprofilemodel.Profile{}, err
	}
	if w.Role == "viewer" {
		return agentruntimeprofilemodel.Profile{}, modelerrors.ErrForbidden
	}
	input.Name = strings.TrimSpace(input.Name)
	create := id == ""
	input.ModelProvider = strings.TrimSpace(input.ModelProvider)
	input.ModelIdentifier = strings.TrimSpace(input.ModelIdentifier)
	input.Configuration = coreruntime.NormalizeConfiguration(input.Configuration)
	if input.Name == "" || len(input.Name) > 200 || input.ModelProvider == "" || len(input.ModelProvider) > 200 || input.ModelIdentifier == "" || len(input.ModelIdentifier) > 200 || !input.Runtime.Available() {
		return agentruntimeprofilemodel.Profile{}, modelerrors.ErrInvalid
	}
	if provider, ok := coreruntime.ProviderFor(input.Runtime); ok {
		if err := provider.ValidateDefinition(coreruntime.Definition{ModelProvider: input.ModelProvider}); err != nil {
			return agentruntimeprofilemodel.Profile{}, modelerrors.ErrInvalid
		}
	} else {
		return agentruntimeprofilemodel.Profile{}, modelerrors.ErrInvalid
	}
	if err := coreruntime.ValidateConfiguration(input.Runtime, input.Configuration); err != nil {
		return agentruntimeprofilemodel.Profile{}, modelerrors.ErrInvalid
	}
	if !create {
		current, getErr := s.repository.Get(ctx, workspaceID, id)
		if getErr != nil {
			return agentruntimeprofilemodel.Profile{}, getErr
		}
		merged := make(map[string]string)
		provider, _ := coreruntime.ProviderFor(input.Runtime)
		allowed := make(map[string]bool)
		for _, option := range provider.ConfigurationSchema() {
			allowed[option.Name] = true
		}
		for key, value := range current.Configuration {
			if allowed[key] {
				merged[key] = value
			}
		}
		for key, value := range input.Configuration {
			if value != "" {
				merged[key] = value
			}
		}
		input.Configuration = merged
	}
	input.HasConfiguration = len(input.Configuration) > 0
	if create {
		id, err = ids.NewUUID()
		if err != nil {
			return agentruntimeprofilemodel.Profile{}, err
		}
	} else {
		if _, err := s.repository.Get(ctx, workspaceID, id); err != nil {
			return agentruntimeprofilemodel.Profile{}, err
		}
	}
	input.ID, input.WorkspaceID = id, workspaceID
	return s.repository.Save(ctx, input, create)
}

func (s *AgentRuntimeProfileService) Delete(ctx context.Context, userID, orgID, workspaceID, id string) error {
	w, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return err
	}
	if w.Role == "viewer" {
		return modelerrors.ErrForbidden
	}
	return s.repository.Delete(ctx, workspaceID, id)
}

// ResolveAgentProfile returns the current model settings so agent updates can snapshot them.
func (s *AgentRuntimeProfileService) ResolveAgentProfile(ctx context.Context, workspaceID, id string) (agentmodel.Runtime, string, string, map[string]string, error) {
	p, err := s.repository.Get(ctx, workspaceID, id)
	return p.Runtime, p.ModelProvider, p.ModelIdentifier, p.Configuration, err
}
