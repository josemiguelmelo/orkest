package checkout

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	codemodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
	runmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

type checkoutProvider struct {
	metadata   codemodel.RepositoryMetadata
	credential codemodel.Credentials
	path       string
	branch     string
	publicURL  string
	err        error
}

func (p *checkoutProvider) PrepareAuthenticatedTaskCheckout(_ context.Context, metadata codemodel.RepositoryMetadata, credential codemodel.Credentials, path, branch string) error {
	p.metadata, p.credential, p.path, p.branch = metadata, credential, path, branch
	return p.err
}

func (p *checkoutProvider) PreparePublicTaskCheckout(_ context.Context, url, path, branch string) error {
	p.publicURL, p.path, p.branch = url, path, branch
	return p.err
}

type credentialResolver func(context.Context, string, string) (codemodel.Credentials, error)

func (f credentialResolver) CodingCredential(ctx context.Context, workspaceID, integrationID string) (codemodel.Credentials, error) {
	return f(ctx, workspaceID, integrationID)
}

type authenticatedCheckout func(context.Context, codemodel.RepositoryMetadata, codemodel.Credentials, string, string) error

func (f authenticatedCheckout) PrepareAuthenticatedTaskCheckout(ctx context.Context, metadata codemodel.RepositoryMetadata, credential codemodel.Credentials, path, branch string) error {
	return f(ctx, metadata, credential, path, branch)
}

func TestPreparerSupportsProviderWithoutPublicCheckout(t *testing.T) {
	called := false
	provider := authenticatedCheckout(func(context.Context, codemodel.RepositoryMetadata, codemodel.Credentials, string, string) error {
		called = true
		return nil
	})
	preparer := Preparer{
		Providers: map[string]Provider{"provider": provider},
		Credentials: credentialResolver(func(context.Context, string, string) (codemodel.Credentials, error) {
			return codemodel.Credentials{Token: "test-token"}, nil
		}),
	}
	repository := runmodel.WorkspaceRepository{ID: "repo", ProviderName: "provider", IntegrationID: "integration"}
	if err := preparer.Prepare(context.Background(), t.TempDir(), "workspace", []runmodel.WorkspaceRepository{repository}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("authenticated checkout was not prepared")
	}
	called = false
	repository.IntegrationID = ""
	err := preparer.Prepare(context.Background(), t.TempDir(), "workspace", []runmodel.WorkspaceRepository{repository})
	if err == nil || called {
		t.Fatal("public checkout must fail without invoking authenticated checkout")
	}
}

func TestPreparerPreservesCheckoutIdentityAndBranchSelection(t *testing.T) {
	for _, integrated := range []bool{false, true} {
		provider := &checkoutProvider{}
		preparer := Preparer{
			Providers: map[string]Provider{"github": provider},
			Credentials: credentialResolver(func(_ context.Context, workspaceID, integrationID string) (codemodel.Credentials, error) {
				if workspaceID != "workspace" || integrationID != "integration" {
					t.Fatal("credential scope changed")
				}
				return codemodel.Credentials{Token: "test-token"}, nil
			}),
		}
		repository := runmodel.WorkspaceRepository{ID: "repo", ProviderName: "github", URL: "https://example.test/repo", Name: "Repository", ProviderRepositoryID: "remote", DefaultBranch: "main"}
		if integrated {
			repository.IntegrationID = "integration"
		}
		root := t.TempDir()
		if err := preparer.Prepare(context.Background(), root, "workspace", []runmodel.WorkspaceRepository{repository}); err != nil {
			t.Fatal(err)
		}
		if provider.path != filepath.Join(root, "repositories", "github", "repo") || provider.branch != "" {
			t.Fatal("checkout path or preserved branch selection changed")
		}
		if integrated {
			if provider.metadata.ID != "remote" || provider.metadata.CloneURL != repository.URL || provider.metadata.DefaultBranch != "main" || provider.credential.Token != "test-token" {
				t.Fatal("authenticated checkout inputs changed")
			}
		} else if provider.publicURL != repository.URL {
			t.Fatal("public checkout inputs changed")
		}
	}
}

func TestPreparerRejectsTraversalAndPreservesErrors(t *testing.T) {
	provider := &checkoutProvider{}
	preparer := Preparer{Providers: map[string]Provider{"github": provider}}
	for _, id := range []string{"", ".", "..", "../escape", "nested/repo"} {
		err := preparer.Prepare(context.Background(), t.TempDir(), "workspace", []runmodel.WorkspaceRepository{{ID: id, ProviderName: "github"}})
		if err == nil || provider.path != "" {
			t.Fatalf("unsafe repository %q reached provider", id)
		}
	}
	cause := errors.New("checkout failed")
	provider.err = cause
	if err := preparer.Prepare(context.Background(), t.TempDir(), "workspace", []runmodel.WorkspaceRepository{{ID: "repo", ProviderName: "github"}}); !errors.Is(err, cause) {
		t.Fatal("provider cause lost")
	}
}
