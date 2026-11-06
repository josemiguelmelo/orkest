package coderepository

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/secretbox"
	coderepositorymodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error)
}

type CodeRepositoryService struct {
	repository  CodeReposRepository
	workspaces  WorkspaceAccess
	dataRoot    string
	providers   map[string]CodingProvider
	secretBox   *secretbox.Box
	operationMu sync.Mutex
}

func NewCodeRepositoryService(repository CodeReposRepository, workspaces WorkspaceAccess, dataRoot string, box *secretbox.Box, providers ...CodingProvider) *CodeRepositoryService {
	providerMap := make(map[string]CodingProvider, len(providers))
	for _, provider := range providers {
		providerMap[provider.ProviderName()] = provider
	}
	return &CodeRepositoryService{repository: repository, workspaces: workspaces, dataRoot: dataRoot, providers: providerMap, secretBox: box}
}

func (s *CodeRepositoryService) List(ctx context.Context, userID, orgID, workspaceID string) ([]coderepositorymodel.CodeRepository, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	return s.repository.CodeRepositories(ctx, orgID, workspaceID)
}

func (s *CodeRepositoryService) Get(ctx context.Context, userID, orgID, workspaceID, id string) (coderepositorymodel.CodeRepository, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return coderepositorymodel.CodeRepository{}, err
	}
	return s.repository.CodeRepository(ctx, orgID, workspaceID, id)
}

func (s *CodeRepositoryService) Delete(ctx context.Context, userID, orgID, workspaceID, id string) error {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return err
	}
	return s.repository.DeleteCodeRepository(ctx, orgID, workspaceID, id)
}

func (s *CodeRepositoryService) Clone(ctx context.Context, userID, orgID, workspaceID, id string) error {
	return s.runOperation(ctx, userID, orgID, workspaceID, id, true)
}

func (s *CodeRepositoryService) Sync(ctx context.Context, userID, orgID, workspaceID, id string) error {
	return s.runOperation(ctx, userID, orgID, workspaceID, id, false)
}

func (s *CodeRepositoryService) runOperation(ctx context.Context, userID, orgID, workspaceID, id string, clone bool) error {
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
		metadata := coderepositorymodel.RepositoryMetadata{ID: repository.ProviderRepositoryID, Name: repository.Name, CloneURL: repository.URL, DefaultBranch: repository.DefaultBranch, Private: repository.Private}
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

func (s *CodeRepositoryService) authorize(ctx context.Context, userID, orgID, workspaceID string) error {
	w, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return err
	}
	if w.Role == "viewer" {
		return modelerrors.ErrForbidden
	}
	return nil
}
