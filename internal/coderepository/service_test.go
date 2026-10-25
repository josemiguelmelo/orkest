package coderepository

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/workspace"
)

type repositoryStub struct {
	items       []CodeRepository
	created     CodeRepository
	createError error
}

func (r *repositoryStub) CodeRepositories(context.Context, string, string) ([]CodeRepository, error) {
	return r.items, nil
}

func (r *repositoryStub) CodeRepository(_ context.Context, _, _, id string) (CodeRepository, error) {
	for _, item := range r.items {
		if item.ID == id {
			return item, nil
		}
	}
	return CodeRepository{}, modelerrors.ErrNotFound
}

func (r *repositoryStub) CreateCodeRepository(_ context.Context, id, workspaceID, providerName, repositoryURL string) (CodeRepository, error) {
	if r.createError != nil {
		return CodeRepository{}, r.createError
	}
	r.created = CodeRepository{ID: id, WorkspaceID: workspaceID, ProviderName: providerName, URL: repositoryURL}
	return r.created, nil
}

func (*repositoryStub) DeleteCodeRepository(context.Context, string, string, string) error {
	return nil
}

type workspaceStub struct{ role string }

func (w workspaceStub) GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error) {
	return workspace.Workspace{Role: w.role}, nil
}

type providerStub struct {
	cloneURL, clonePath string
	syncURL, syncPath   string
}

func (providerStub) ProviderName() string { return "github" }
func (p *providerStub) Clone(_ context.Context, repositoryURL, path string) error {
	p.cloneURL, p.clonePath = repositoryURL, path
	return nil
}

func (p *providerStub) SyncMain(_ context.Context, repositoryURL, path string) error {
	p.syncURL, p.syncPath = repositoryURL, path
	return nil
}

func TestCreateNormalizesGitHubURLAndSetsProviderName(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository, workspaceStub{role: "member"}, t.TempDir(), &providerStub{})
	created, err := service.Create(context.Background(), "user", "org", "workspace", "github", " https://github.com/acme/project/ ")
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if created.ProviderName != "github" || created.URL != "https://github.com/acme/project.git" || created.ID == "" {
		t.Fatalf("unexpected repository: %+v", created)
	}
}

func TestCreateRejectsUnsupportedProviderAndNonGitHubURL(t *testing.T) {
	service := NewService(&repositoryStub{}, workspaceStub{role: "member"}, t.TempDir(), &providerStub{})
	for _, tc := range []struct{ providerName, repositoryURL string }{
		{"gitlab", "https://github.com/acme/project"},
		{"github", "https://example.com/acme/project"},
		{"github", "file:///tmp/repository"},
	} {
		if _, err := service.Create(context.Background(), "user", "org", "workspace", tc.providerName, tc.repositoryURL); !errors.Is(err, modelerrors.ErrInvalid) {
			t.Errorf("Create(%q, %q) error = %v, want invalid request", tc.providerName, tc.repositoryURL, err)
		}
	}
}

func TestViewerCannotMutateRepositories(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository, workspaceStub{role: "viewer"}, t.TempDir(), &providerStub{})
	if _, err := service.Create(context.Background(), "user", "org", "workspace", "github", "https://github.com/acme/project"); !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatalf("Create error = %v, want forbidden", err)
	}
	if repository.created.ID != "" {
		t.Fatal("viewer mutation reached repository")
	}
}

func TestCloneAndSyncUseRegisteredProviderAndRequestedPath(t *testing.T) {
	provider := &providerStub{}
	repository := &repositoryStub{items: []CodeRepository{{ID: "repo", ProviderName: "github", URL: "https://github.com/acme/project.git"}}}
	dataRoot := t.TempDir()
	service := NewService(repository, workspaceStub{role: "member"}, dataRoot, provider)
	if err := service.Clone(context.Background(), "user", "org", "workspace", "repo"); err != nil {
		t.Fatalf("clone: %v", err)
	}
	expectedPath := filepath.Join(dataRoot, "org", "workspace", "git", "github", "repo")
	if provider.cloneURL != repository.items[0].URL || provider.clonePath != expectedPath {
		t.Fatalf("unexpected clone arguments: url=%q path=%q", provider.cloneURL, provider.clonePath)
	}
	if err := service.SyncMain(context.Background(), "user", "org", "workspace", "repo"); err != nil {
		t.Fatalf("sync main: %v", err)
	}
	if provider.syncURL != repository.items[0].URL || provider.syncPath != expectedPath {
		t.Fatalf("unexpected sync arguments: url=%q path=%q", provider.syncURL, provider.syncPath)
	}
}
