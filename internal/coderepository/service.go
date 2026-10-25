package coderepository

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/core/ids"
	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/workspace"
)

type WorkspaceAccess interface {
	GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error)
}

type Service struct {
	repository CodeReposRepository
	workspaces WorkspaceAccess
	dataRoot   string
	providers  map[string]CodeRepositoryProvider
}

func NewService(repository CodeReposRepository, workspaces WorkspaceAccess, dataRoot string, providers ...CodeRepositoryProvider) *Service {
	providerMap := make(map[string]CodeRepositoryProvider, len(providers))
	for _, provider := range providers {
		providerMap[provider.ProviderName()] = provider
	}
	return &Service{repository: repository, workspaces: workspaces, dataRoot: dataRoot, providers: providerMap}
}

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

func (s *Service) Create(ctx context.Context, userID, orgID, workspaceID, providerName, rawURL string) (CodeRepository, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return CodeRepository{}, err
	}
	providerName = strings.TrimSpace(providerName)
	provider, ok := s.providers[providerName]
	if !ok || providerName == "" {
		return CodeRepository{}, modelerrors.ErrInvalid
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || strings.Trim(parsed.Path, "/") == "" {
		return CodeRepository{}, modelerrors.ErrInvalid
	}
	repositoryURL := "https://github.com/" + strings.Trim(strings.TrimSpace(parsed.Path), "/")
	if !strings.HasSuffix(repositoryURL, ".git") {
		repositoryURL += ".git"
	}
	id, err := ids.NewUUID()
	if err != nil {
		return CodeRepository{}, fmt.Errorf("generate code repository ID: %w", err)
	}
	return s.repository.CreateCodeRepository(ctx, id, workspaceID, provider.ProviderName(), repositoryURL)
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

func (s *Service) SyncMain(ctx context.Context, userID, orgID, workspaceID, id string) error {
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
	if clone {
		return provider.Clone(ctx, repository.URL, path)
	}
	return provider.SyncMain(ctx, repository.URL, path)
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
