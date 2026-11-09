package checkout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	observation "github.com/josemiguelmelo/orkest/internal/contracts/repositoryobservation"
	"github.com/josemiguelmelo/orkest/internal/platform/durablefile"

	coderepositorymodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

type CredentialResolver interface {
	CodingCredential(context.Context, string, string) (coderepositorymodel.Credentials, error)
}

// Provider prepares authenticated checkouts without requiring repository discovery.
type Provider interface {
	PrepareAuthenticatedTaskCheckout(context.Context, coderepositorymodel.RepositoryMetadata, coderepositorymodel.Credentials, string, string) error
}

// PublicProvider optionally supports existing public repository registrations.
type PublicProvider interface {
	PreparePublicTaskCheckout(context.Context, string, string, string) error
}

// Preparer checks out registered repositories without replacing preserved task branches.
type Preparer struct {
	Providers   map[string]Provider
	Credentials CredentialResolver
}

func (p Preparer) Prepare(ctx context.Context, workDir, workspaceID string, repositories []orchestrationmodel.WorkspaceRepository) error {
	for _, repository := range repositories {
		if err := p.prepare(ctx, workDir, workspaceID, repository); err != nil {
			return err
		}
	}
	manifest := make([]observation.Checkout, 0, len(repositories))
	for _, repository := range repositories {
		manifest = append(manifest, observation.Checkout{RepositoryID: repository.ID, Provider: repository.ProviderName, ProviderRepositoryID: repository.ProviderRepositoryID, Path: filepath.Join("repositories", repository.ProviderName, repository.ID), DefaultBranch: repository.DefaultBranch, RepositoryURL: repository.URL})
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode checkout manifest: %w", err)
	}
	return durablefile.Write(filepath.Join(workDir, observation.ManifestName), data)
}

func (p Preparer) prepare(ctx context.Context, workDir, workspaceID string, repository orchestrationmodel.WorkspaceRepository) error {
	provider, ok := p.Providers[repository.ProviderName]
	if !ok {
		return fmt.Errorf("repository provider %q is unavailable", repository.ProviderName)
	}
	if !safePathComponent(repository.ProviderName) || !safePathComponent(repository.ID) {
		return errors.New("repository identity is invalid")
	}
	path := filepath.Join(workDir, "repositories", repository.ProviderName, repository.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create repository directory: %w", err)
	}
	if err := p.checkout(ctx, provider, workspaceID, repository, path); err != nil {
		return fmt.Errorf("prepare repository %q: %w", repository.ID, err)
	}
	return nil
}

func (p Preparer) checkout(ctx context.Context, provider Provider, workspaceID string, repository orchestrationmodel.WorkspaceRepository, path string) error {
	if repository.IntegrationID == "" {
		legacy, ok := provider.(PublicProvider)
		if !ok {
			return errors.New("legacy repository checkout is unavailable")
		}
		return legacy.PreparePublicTaskCheckout(ctx, repository.URL, path, "")
	}
	if p.Credentials == nil {
		return errors.New("coding credential resolver is unavailable")
	}
	credential, err := p.Credentials.CodingCredential(ctx, workspaceID, repository.IntegrationID)
	if err != nil {
		return fmt.Errorf("resolve coding integration credential: %w", err)
	}
	metadata := coderepositorymodel.RepositoryMetadata{ID: repository.ProviderRepositoryID, Name: repository.Name, CloneURL: repository.URL, DefaultBranch: repository.DefaultBranch}
	return provider.PrepareAuthenticatedTaskCheckout(ctx, metadata, credential, path, "")
}

func safePathComponent(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value && !strings.ContainsRune(value, filepath.Separator)
}
