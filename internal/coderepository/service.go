package coderepository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/core/secretbox"
	"github.com/josemiguelmelo/orkest/internal/workspace"
)

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error)
}

type Service struct {
	repository  CodeReposRepository
	workspaces  WorkspaceAccess
	dataRoot    string
	providers   map[string]CodingProvider
	secretBox   *secretbox.Box
	operationMu sync.Mutex
}

func NewService(repository CodeReposRepository, workspaces WorkspaceAccess, dataRoot string, providers ...CodingProvider) *Service {
	providerMap := make(map[string]CodingProvider, len(providers))
	for _, provider := range providers {
		providerMap[provider.ProviderName()] = provider
	}
	return &Service{repository: repository, workspaces: workspaces, dataRoot: dataRoot, providers: providerMap}
}

func (s *Service) SetSecretBox(box *secretbox.Box) { s.secretBox = box }

func (s *Service) List(ctx context.Context, userID, orgID, workspaceID string) ([]CodeRepository, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repository.CodeRepositories(ctx, orgID, workspaceID)
}

func (s *Service) Get(ctx context.Context, userID, orgID, workspaceID, id string) (CodeRepository, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return CodeRepository{}, err
	}
	return s.repository.CodeRepository(ctx, orgID, workspaceID, id)
}

func (s *Service) Delete(ctx context.Context, userID, orgID, workspaceID, id string) error {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return err
	}
	return s.repository.DeleteCodeRepository(ctx, orgID, workspaceID, id)
}

func (s *Service) Clone(ctx context.Context, userID, orgID, workspaceID, id string) error {
	return s.runOperation(ctx, userID, orgID, workspaceID, id, true)
}

func (s *Service) Sync(ctx context.Context, userID, orgID, workspaceID, id string) error {
	return s.runOperation(ctx, userID, orgID, workspaceID, id, false)
}

func (s *Service) runOperation(ctx context.Context, userID, orgID, workspaceID, id string, clone bool) error {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return err
	}
	if !filepath.IsAbs(s.dataRoot) {
		return modelerrors.ErrInvalid
	}
	repository, err := s.repository.CodeRepository(ctx, orgID, workspaceID, id)
	if err != nil {
		return err
	}
	provider, ok := s.providers[repository.ProviderName]
	if !ok {
		return modelerrors.ErrInvalid
	}
	path, err := repositoryPath(s.dataRoot, orgID, workspaceID, repository.ProviderName, repository.ID)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create workspace repository directory: %w", err)
	}
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if repository.IntegrationID != "" {
		credential, credentialErr := s.CodingCredential(ctx, workspaceID, repository.IntegrationID)
		if credentialErr != nil {
			return credentialErr
		}
		metadata := RepositoryMetadata{ID: repository.ProviderRepositoryID, Name: repository.Name, CloneURL: repository.URL, DefaultBranch: repository.DefaultBranch, Private: repository.Private}
		if clone {
			return provider.CloneAuthenticated(ctx, metadata, credential, path)
		}
		return provider.SyncAuthenticated(ctx, metadata, credential, path)
	}
	if clone {
		legacy, ok := provider.(LegacyRepositoryCheckout)
		if !ok {
			return errors.New("legacy repository checkout is unavailable")
		}
		return legacy.ClonePublic(ctx, repository.URL, path)
	}
	legacy, ok := provider.(LegacyRepositoryCheckout)
	if !ok {
		return errors.New("legacy repository checkout is unavailable")
	}
	return legacy.SyncPublic(ctx, repository.URL, path)
}

func repositoryPath(dataRoot, organizationID, workspaceID, providerName, repositoryID string) (string, error) {
	for _, component := range []string{organizationID, workspaceID, providerName, repositoryID} {
		if component == "" || component == "." || component == ".." || filepath.Base(component) != component || strings.ContainsRune(component, filepath.Separator) {
			return "", modelerrors.ErrInvalid
		}
	}
	return filepath.Join(dataRoot, organizationID, workspaceID, "git", providerName, repositoryID), nil
}

func (s *Service) authorize(ctx context.Context, userID, orgID, workspaceID string) error {
	w, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return err
	}
	if w.Role == "viewer" {
		return modelerrors.ErrForbidden
	}
	return nil
}
