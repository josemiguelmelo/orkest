package agentruntimeprofile

import (
	"context"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/agent"
	"github.com/josemiguelmelo/orkest/internal/core/ids"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	coreruntime "github.com/josemiguelmelo/orkest/internal/core/runtime"
	"github.com/josemiguelmelo/orkest/internal/workspace"
)

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error)
}

type Service struct {
	repository Repository
	workspaces WorkspaceAccess
}

func NewService(repository Repository, workspaces WorkspaceAccess) *Service {
	return &Service{repository: repository, workspaces: workspaces}
}

func (s *Service) List(ctx context.Context, userID, orgID, workspaceID string) ([]Profile, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repository.List(ctx, workspaceID)
}

func (s *Service) Save(ctx context.Context, userID, orgID, workspaceID, id string, input Profile) (Profile, error) {
	w, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return Profile{}, err
	}
	if w.Role == "viewer" {
		return Profile{}, modelerrors.ErrForbidden
	}
	input.Name = strings.TrimSpace(input.Name)
	input.ModelProvider = strings.TrimSpace(input.ModelProvider)
	input.ModelIdentifier = strings.TrimSpace(input.ModelIdentifier)
	if input.Name == "" || len(input.Name) > 200 || input.ModelProvider == "" || len(input.ModelProvider) > 200 || input.ModelIdentifier == "" || len(input.ModelIdentifier) > 200 || !input.Runtime.Available() {
		return Profile{}, modelerrors.ErrInvalid
	}
	if provider, ok := coreruntime.ProviderFor(input.Runtime); ok {
		if err := provider.ValidateDefinition(coreruntime.Definition{ModelProvider: input.ModelProvider}); err != nil {
			return Profile{}, modelerrors.ErrInvalid
		}
	} else {
		return Profile{}, modelerrors.ErrInvalid
	}
	create := id == ""
	if create {
		id, err = ids.NewUUID()
		if err != nil {
			return Profile{}, err
		}
	} else {
		if _, err := s.repository.Get(ctx, workspaceID, id); err != nil {
			return Profile{}, err
		}
	}
	input.ID, input.WorkspaceID = id, workspaceID
	return s.repository.Save(ctx, input, create)
}

func (s *Service) Delete(ctx context.Context, userID, orgID, workspaceID, id string) error {
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
func (s *Service) ResolveAgentProfile(ctx context.Context, workspaceID, id string) (agent.Runtime, string, string, error) {
	p, err := s.repository.Get(ctx, workspaceID, id)
	return p.Runtime, p.ModelProvider, p.ModelIdentifier, err
}
