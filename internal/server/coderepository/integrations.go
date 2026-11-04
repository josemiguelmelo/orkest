package coderepository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/ids"
	"github.com/josemiguelmelo/orkest/internal/platform/secretbox"
)

type CodingProviderInfo struct {
	ProviderName string          `json:"providerName"`
	ConfigSchema json.RawMessage `json:"configSchema"`
}

func (s *CodeRepositoryService) ProviderCatalog() []CodingProviderInfo {
	out := make([]CodingProviderInfo, 0, len(s.providers))
	for name, provider := range s.providers {
		out = append(out, CodingProviderInfo{ProviderName: name, ConfigSchema: provider.ConfigSchema()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ProviderName < out[j].ProviderName })
	return out
}

func (s *CodeRepositoryService) ListIntegrations(ctx context.Context, userID, orgID, workspaceID string) ([]CodingIntegration, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	repository, err := s.integrationRepository()
	if err != nil {
		return nil, err
	}
	return repository.ListCodingIntegrations(ctx, workspaceID)
}

func (s *CodeRepositoryService) authorizeIntegrationAdmin(ctx context.Context, userID, orgID, workspaceID string) error {
	w, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return err
	}
	if w.Role != "admin" && w.Role != "owner" {
		return modelerrors.ErrForbidden
	}
	return nil
}

func (s *CodeRepositoryService) CreateIntegration(ctx context.Context, userID, orgID, workspaceID string, input CodingIntegrationInput) (CodingIntegration, error) {
	if err := s.authorizeIntegrationAdmin(ctx, userID, orgID, workspaceID); err != nil {
		return CodingIntegration{}, err
	}
	box := s.secretBox
	if box == nil {
		return CodingIntegration{}, secretbox.ErrKeyUnavailable
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Credential = strings.TrimSpace(input.Credential)
	if input.Name == "" || len(input.Name) > 200 || strings.TrimSpace(input.Credential) == "" {
		return CodingIntegration{}, modelerrors.ErrInvalid
	}
	provider, ok := s.providers[strings.TrimSpace(input.ProviderName)]
	if !ok {
		return CodingIntegration{}, modelerrors.ErrInvalid
	}
	if len(input.ProviderConfig) == 0 {
		input.ProviderConfig = json.RawMessage(`{}`)
	}
	if err := provider.ValidateConfig(input.ProviderConfig); err != nil {
		return CodingIntegration{}, modelerrors.ErrInvalid
	}
	if err := provider.ValidateAuthentication(ctx, input.ProviderConfig, Credentials{Token: input.Credential}); err != nil {
		return CodingIntegration{}, err
	}
	sealed, err := box.Seal(input.Credential)
	if err != nil {
		return CodingIntegration{}, fmt.Errorf("encrypt coding integration credential: %w", err)
	}
	id, err := ids.NewUUID()
	if err != nil {
		return CodingIntegration{}, fmt.Errorf("generate coding integration ID: %w", err)
	}
	repository, err := s.integrationRepository()
	if err != nil {
		return CodingIntegration{}, err
	}
	return repository.CreateCodingIntegration(ctx, CodingIntegration{ID: id, WorkspaceID: workspaceID, Name: input.Name, ProviderName: provider.ProviderName(), ProviderConfig: input.ProviderConfig}, string(sealed))
}

func (s *CodeRepositoryService) UpdateIntegration(ctx context.Context, userID, orgID, workspaceID, id string, input CodingIntegrationInput) error {
	if err := s.authorizeIntegrationAdmin(ctx, userID, orgID, workspaceID); err != nil {
		return err
	}
	box := s.secretBox
	if box == nil {
		return secretbox.ErrKeyUnavailable
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Credential = strings.TrimSpace(input.Credential)
	if input.Name == "" || len(input.Name) > 200 {
		return modelerrors.ErrInvalid
	}
	repository, err := s.integrationRepository()
	if err != nil {
		return err
	}
	old, sealed, err := repository.GetCodingIntegration(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	if input.ProviderName != "" && input.ProviderName != old.ProviderName {
		return modelerrors.ErrInvalid
	}
	if len(input.ProviderConfig) == 0 {
		input.ProviderConfig = old.ProviderConfig
	}
	provider := s.providers[old.ProviderName]
	if provider == nil {
		return modelerrors.ErrInvalid
	}
	if err = provider.ValidateConfig(input.ProviderConfig); err != nil {
		return modelerrors.ErrInvalid
	}
	credential := input.Credential
	if strings.TrimSpace(credential) == "" {
		credential, err = box.Open([]byte(sealed))
		if err != nil {
			return errors.New("stored integration credential is unavailable")
		}
	}
	if err = provider.ValidateAuthentication(ctx, input.ProviderConfig, Credentials{Token: credential}); err != nil {
		return err
	}
	if input.Credential != "" {
		encrypted, e := box.Seal(input.Credential)
		if e != nil {
			return fmt.Errorf("encrypt coding integration credential: %w", e)
		}
		sealed = string(encrypted)
	}
	old.Name = input.Name
	old.ProviderConfig = input.ProviderConfig
	return repository.UpdateCodingIntegration(ctx, old, sealed)
}

func (s *CodeRepositoryService) ValidateIntegration(ctx context.Context, userID, orgID, workspaceID, id string) error {
	if err := s.authorizeIntegrationAdmin(ctx, userID, orgID, workspaceID); err != nil {
		return err
	}
	box := s.secretBox
	if box == nil {
		return secretbox.ErrKeyUnavailable
	}
	repository, err := s.integrationRepository()
	if err != nil {
		return err
	}
	item, sealed, err := repository.GetCodingIntegration(ctx, workspaceID, id)
	if err != nil {
		return err
	}
	credential, err := box.Open([]byte(sealed))
	if err != nil {
		return errors.New("stored integration credential is unavailable")
	}
	provider := s.providers[item.ProviderName]
	if provider == nil {
		return modelerrors.ErrInvalid
	}
	return provider.ValidateAuthentication(ctx, item.ProviderConfig, Credentials{Token: credential})
}

func (s *CodeRepositoryService) DeleteIntegration(ctx context.Context, userID, orgID, workspaceID, id string) error {
	if err := s.authorizeIntegrationAdmin(ctx, userID, orgID, workspaceID); err != nil {
		return err
	}
	repository, err := s.integrationRepository()
	if err != nil {
		return err
	}
	return repository.DeleteCodingIntegration(ctx, workspaceID, id)
}

func (s *CodeRepositoryService) BrowseRepositories(ctx context.Context, userID, orgID, workspaceID, id string, page, perPage int) (RepositoryPage, error) {
	w, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return RepositoryPage{}, err
	}
	if w.Role == "viewer" {
		return RepositoryPage{}, modelerrors.ErrForbidden
	}
	box := s.secretBox
	if box == nil {
		return RepositoryPage{}, secretbox.ErrKeyUnavailable
	}
	repository, err := s.integrationRepository()
	if err != nil {
		return RepositoryPage{}, err
	}
	item, sealed, err := repository.GetCodingIntegration(ctx, workspaceID, id)
	if err != nil {
		return RepositoryPage{}, err
	}
	token, err := box.Open([]byte(sealed))
	if err != nil {
		return RepositoryPage{}, errors.New("stored integration credential is unavailable")
	}
	provider := s.providers[item.ProviderName]
	if provider == nil {
		return RepositoryPage{}, modelerrors.ErrInvalid
	}
	return provider.DiscoverRepositories(ctx, item.ProviderConfig, Credentials{Token: token}, page, perPage)
}

func (s *CodeRepositoryService) RegisterRepository(ctx context.Context, userID, orgID, workspaceID string, integrationID, providerRepositoryID string) (CodeRepository, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return CodeRepository{}, err
	}
	if s.secretBox == nil {
		return CodeRepository{}, secretbox.ErrKeyUnavailable
	}
	repo, err := s.integrationRepository()
	if err != nil {
		return CodeRepository{}, err
	}
	integration, sealed, err := repo.GetCodingIntegration(ctx, workspaceID, integrationID)
	if err != nil {
		return CodeRepository{}, err
	}
	provider := s.providers[integration.ProviderName]
	if provider == nil {
		return CodeRepository{}, modelerrors.ErrInvalid
	}
	token, err := s.secretBox.Open([]byte(sealed))
	if err != nil {
		return CodeRepository{}, errors.New("stored integration credential is unavailable")
	}
	metadata, err := provider.ResolveRepository(ctx, integration.ProviderConfig, Credentials{Token: token}, providerRepositoryID)
	if err != nil {
		return CodeRepository{}, err
	}
	cloneURL, err := provider.NormalizeRepositoryURL(metadata.CloneURL)
	if err != nil {
		return CodeRepository{}, modelerrors.ErrInvalid
	}
	if metadata.ID == "" || metadata.DefaultBranch == "" {
		return CodeRepository{}, modelerrors.ErrInvalid
	}
	id, err := ids.NewUUID()
	if err != nil {
		return CodeRepository{}, fmt.Errorf("generate code repository ID: %w", err)
	}
	create, ok := s.repository.(IntegratedCodeRepositoryRepository)
	if !ok {
		return CodeRepository{}, errors.New("integrated repository storage is unavailable")
	}
	item := CodeRepository{ID: id, WorkspaceID: workspaceID, ProviderName: integration.ProviderName, URL: cloneURL, IntegrationID: integrationID, ProviderRepositoryID: metadata.ID, DefaultBranch: metadata.DefaultBranch, Name: metadata.Name, Private: metadata.Private}
	return create.CreateIntegratedCodeRepository(ctx, item)
}

func (s *CodeRepositoryService) CodingCredential(ctx context.Context, workspaceID, integrationID string) (Credentials, error) {
	if s.secretBox == nil {
		return Credentials{}, secretbox.ErrKeyUnavailable
	}
	repo, err := s.integrationRepository()
	if err != nil {
		return Credentials{}, err
	}
	integration, sealed, err := repo.GetCodingIntegration(ctx, workspaceID, integrationID)
	if err != nil {
		return Credentials{}, err
	}
	token, err := s.secretBox.Open([]byte(sealed))
	if err != nil {
		return Credentials{}, errors.New("stored integration credential is unavailable")
	}
	return Credentials{Token: token, ProviderConfig: integration.ProviderConfig}, nil
}

func (s *CodeRepositoryService) integrationRepository() (CodingIntegrationRepository, error) {
	repo, ok := s.repository.(CodingIntegrationRepository)
	if !ok {
		return nil, errors.New("coding integration storage is unavailable")
	}
	return repo, nil
}
