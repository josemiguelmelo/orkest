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
	coderepositorymodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
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

func (s *CodeRepositoryService) ListIntegrations(ctx context.Context, userID, orgID, workspaceID string) ([]coderepositorymodel.CodingIntegration, error) {
	if _, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID); err != nil {
		return nil, err
	}
	repository, err := s.integrationRepository()
	if err != nil {
		return nil, err
	}
	integrations, err := repository.ListCodingIntegrations(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for i := range integrations {
		integrations[i], err = s.withIntegrationBaseURL(integrations[i])
		if err != nil {
			return nil, err
		}
	}
	return integrations, nil
}

// Resolve the base URL from provider configuration, including existing integrations.
func (s *CodeRepositoryService) withIntegrationBaseURL(integration coderepositorymodel.CodingIntegration) (coderepositorymodel.CodingIntegration, error) {
	provider := s.providers[integration.ProviderName]
	if provider == nil {
		return coderepositorymodel.CodingIntegration{}, modelerrors.ErrInvalid
	}
	baseURL, err := provider.BaseURL(integration.ProviderConfig)
	if err != nil {
		return coderepositorymodel.CodingIntegration{}, err
	}
	integration.BaseURL = strings.TrimRight(baseURL, "/") + "/"
	return integration, nil
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

func (s *CodeRepositoryService) CreateIntegration(ctx context.Context, userID, orgID, workspaceID string, input coderepositorymodel.CodingIntegrationInput) (coderepositorymodel.CodingIntegration, error) {
	if err := s.authorizeIntegrationAdmin(ctx, userID, orgID, workspaceID); err != nil {
		return coderepositorymodel.CodingIntegration{}, err
	}
	box := s.secretBox
	if box == nil {
		return coderepositorymodel.CodingIntegration{}, secretbox.ErrKeyUnavailable
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Credential = strings.TrimSpace(input.Credential)
	if input.Name == "" || len(input.Name) > 200 || strings.TrimSpace(input.Credential) == "" {
		return coderepositorymodel.CodingIntegration{}, modelerrors.ErrInvalid
	}
	provider, ok := s.providers[strings.TrimSpace(input.ProviderName)]
	if !ok {
		return coderepositorymodel.CodingIntegration{}, modelerrors.ErrInvalid
	}
	if len(input.ProviderConfig) == 0 {
		input.ProviderConfig = json.RawMessage(`{}`)
	}
	if err := provider.ValidateConfig(input.ProviderConfig); err != nil {
		return coderepositorymodel.CodingIntegration{}, modelerrors.ErrInvalid
	}
	if err := provider.ValidateAuthentication(ctx, input.ProviderConfig, coderepositorymodel.Credentials{Token: input.Credential}); err != nil {
		return coderepositorymodel.CodingIntegration{}, err
	}
	sealed, err := box.Seal(input.Credential)
	if err != nil {
		return coderepositorymodel.CodingIntegration{}, fmt.Errorf("encrypt coding integration credential: %w", err)
	}
	id, err := ids.NewUUID()
	if err != nil {
		return coderepositorymodel.CodingIntegration{}, fmt.Errorf("generate coding integration ID: %w", err)
	}
	repository, err := s.integrationRepository()
	if err != nil {
		return coderepositorymodel.CodingIntegration{}, err
	}
	integration, err := repository.CreateCodingIntegration(ctx, coderepositorymodel.CodingIntegration{ID: id, WorkspaceID: workspaceID, Name: input.Name, ProviderName: provider.ProviderName(), ProviderConfig: input.ProviderConfig}, string(sealed))
	if err != nil {
		return coderepositorymodel.CodingIntegration{}, err
	}
	return s.withIntegrationBaseURL(integration)
}

func (s *CodeRepositoryService) UpdateIntegration(ctx context.Context, userID, orgID, workspaceID, id string, input coderepositorymodel.CodingIntegrationInput) error {
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
	if err = provider.ValidateAuthentication(ctx, input.ProviderConfig, coderepositorymodel.Credentials{Token: credential}); err != nil {
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
	return provider.ValidateAuthentication(ctx, item.ProviderConfig, coderepositorymodel.Credentials{Token: credential})
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

func (s *CodeRepositoryService) BrowseRepositories(ctx context.Context, userID, orgID, workspaceID, id string, page, perPage int, search string) (coderepositorymodel.RepositoryPage, error) {
	w, err := s.workspaces.GetWorkspace(ctx, userID, orgID, workspaceID)
	if err != nil {
		return coderepositorymodel.RepositoryPage{}, err
	}
	if w.Role == "viewer" {
		return coderepositorymodel.RepositoryPage{}, modelerrors.ErrForbidden
	}
	box := s.secretBox
	if box == nil {
		return coderepositorymodel.RepositoryPage{}, secretbox.ErrKeyUnavailable
	}
	repository, err := s.integrationRepository()
	if err != nil {
		return coderepositorymodel.RepositoryPage{}, err
	}
	item, sealed, err := repository.GetCodingIntegration(ctx, workspaceID, id)
	if err != nil {
		return coderepositorymodel.RepositoryPage{}, err
	}
	token, err := box.Open([]byte(sealed))
	if err != nil {
		return coderepositorymodel.RepositoryPage{}, errors.New("stored integration credential is unavailable")
	}
	provider := s.providers[item.ProviderName]
	if provider == nil {
		return coderepositorymodel.RepositoryPage{}, modelerrors.ErrInvalid
	}
	if strings.TrimSpace(search) == "" {
		return provider.DiscoverRepositories(ctx, item.ProviderConfig, coderepositorymodel.Credentials{Token: token}, page, perPage)
	}
	search = strings.ToLower(strings.TrimSpace(search))
	matched := make([]coderepositorymodel.RepositoryMetadata, 0)
	for providerPage := 1; providerPage > 0; {
		result, discoverErr := provider.DiscoverRepositories(ctx, item.ProviderConfig, coderepositorymodel.Credentials{Token: token}, providerPage, 100)
		if discoverErr != nil {
			return coderepositorymodel.RepositoryPage{}, discoverErr
		}
		for _, repository := range result.Repositories {
			if strings.Contains(strings.ToLower(repository.Name), search) {
				matched = append(matched, repository)
			}
		}
		providerPage = result.NextPage
	}
	start := (page - 1) * perPage
	if start >= len(matched) {
		return coderepositorymodel.RepositoryPage{Repositories: []coderepositorymodel.RepositoryMetadata{}}, nil
	}
	end := start + perPage
	if end > len(matched) {
		end = len(matched)
	}
	next := 0
	if end < len(matched) {
		next = page + 1
	}
	return coderepositorymodel.RepositoryPage{Repositories: matched[start:end], NextPage: next}, nil
}

func (s *CodeRepositoryService) RegisterRepository(ctx context.Context, userID, orgID, workspaceID string, integrationID, providerRepositoryID string) (coderepositorymodel.CodeRepository, error) {
	if err := s.authorize(ctx, userID, orgID, workspaceID); err != nil {
		return coderepositorymodel.CodeRepository{}, err
	}
	if s.secretBox == nil {
		return coderepositorymodel.CodeRepository{}, secretbox.ErrKeyUnavailable
	}
	repo, err := s.integrationRepository()
	if err != nil {
		return coderepositorymodel.CodeRepository{}, err
	}
	integration, sealed, err := repo.GetCodingIntegration(ctx, workspaceID, integrationID)
	if err != nil {
		return coderepositorymodel.CodeRepository{}, err
	}
	provider := s.providers[integration.ProviderName]
	if provider == nil {
		return coderepositorymodel.CodeRepository{}, modelerrors.ErrInvalid
	}
	token, err := s.secretBox.Open([]byte(sealed))
	if err != nil {
		return coderepositorymodel.CodeRepository{}, errors.New("stored integration credential is unavailable")
	}
	metadata, err := provider.ResolveRepository(ctx, integration.ProviderConfig, coderepositorymodel.Credentials{Token: token}, providerRepositoryID)
	if err != nil {
		return coderepositorymodel.CodeRepository{}, err
	}
	cloneURL, err := provider.NormalizeRepositoryURL(metadata.CloneURL)
	if err != nil {
		return coderepositorymodel.CodeRepository{}, modelerrors.ErrInvalid
	}
	if metadata.ID == "" || metadata.DefaultBranch == "" {
		return coderepositorymodel.CodeRepository{}, modelerrors.ErrInvalid
	}
	id, err := ids.NewUUID()
	if err != nil {
		return coderepositorymodel.CodeRepository{}, fmt.Errorf("generate code repository ID: %w", err)
	}
	create, ok := s.repository.(IntegratedCodeRepositoryRepository)
	if !ok {
		return coderepositorymodel.CodeRepository{}, errors.New("integrated repository storage is unavailable")
	}
	item := coderepositorymodel.CodeRepository{ID: id, WorkspaceID: workspaceID, ProviderName: integration.ProviderName, URL: cloneURL, IntegrationID: integrationID, ProviderRepositoryID: metadata.ID, DefaultBranch: metadata.DefaultBranch, Name: metadata.Name, Private: metadata.Private}
	return create.CreateIntegratedCodeRepository(ctx, item)
}

func (s *CodeRepositoryService) CodingCredential(ctx context.Context, workspaceID, integrationID string) (coderepositorymodel.Credentials, error) {
	if s.secretBox == nil {
		return coderepositorymodel.Credentials{}, secretbox.ErrKeyUnavailable
	}
	repo, err := s.integrationRepository()
	if err != nil {
		return coderepositorymodel.Credentials{}, err
	}
	integration, sealed, err := repo.GetCodingIntegration(ctx, workspaceID, integrationID)
	if err != nil {
		return coderepositorymodel.Credentials{}, err
	}
	token, err := s.secretBox.Open([]byte(sealed))
	if err != nil {
		return coderepositorymodel.Credentials{}, errors.New("stored integration credential is unavailable")
	}
	return coderepositorymodel.Credentials{Token: token, ProviderConfig: integration.ProviderConfig}, nil
}

func (s *CodeRepositoryService) integrationRepository() (CodingIntegrationRepository, error) {
	repo, ok := s.repository.(CodingIntegrationRepository)
	if !ok {
		return nil, errors.New("coding integration storage is unavailable")
	}
	return repo, nil
}
