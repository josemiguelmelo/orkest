package coderepository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	"github.com/josemiguelmelo/orkest/internal/core/secretbox"
	"github.com/josemiguelmelo/orkest/internal/workspace"
)

type repositoryStub struct {
	items        []CodeRepository
	integrations map[string]CodingIntegration
	credentials  map[string]string
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

func (*repositoryStub) DeleteCodeRepository(context.Context, string, string, string) error {
	return nil
}

func (r *repositoryStub) ListCodingIntegrations(_ context.Context, workspaceID string) ([]CodingIntegration, error) {
	out := []CodingIntegration{}
	for _, item := range r.integrations {
		if item.WorkspaceID == workspaceID {
			out = append(out, item)
		}
	}
	return out, nil
}

func (r *repositoryStub) GetCodingIntegration(_ context.Context, workspaceID, id string) (CodingIntegration, string, error) {
	item, ok := r.integrations[id]
	if !ok || item.WorkspaceID != workspaceID {
		return CodingIntegration{}, "", modelerrors.ErrNotFound
	}
	return item, r.credentials[id], nil
}

func (r *repositoryStub) CreateCodingIntegration(_ context.Context, item CodingIntegration, credential string) (CodingIntegration, error) {
	if r.integrations == nil {
		r.integrations = map[string]CodingIntegration{}
	}
	if r.credentials == nil {
		r.credentials = map[string]string{}
	}
	r.integrations[item.ID] = item
	r.credentials[item.ID] = credential
	return item, nil
}

func (r *repositoryStub) UpdateCodingIntegration(_ context.Context, item CodingIntegration, credential string) error {
	if _, ok := r.integrations[item.ID]; !ok {
		return modelerrors.ErrNotFound
	}
	r.integrations[item.ID] = item
	r.credentials[item.ID] = credential
	return nil
}

func (r *repositoryStub) DeleteCodingIntegration(_ context.Context, workspaceID, id string) error {
	for _, repo := range r.items {
		if repo.IntegrationID == id {
			return modelerrors.ErrConflict
		}
	}
	if item, ok := r.integrations[id]; ok && item.WorkspaceID == workspaceID {
		delete(r.integrations, id)
		delete(r.credentials, id)
		return nil
	}
	return modelerrors.ErrNotFound
}

func (r *repositoryStub) CreateIntegratedCodeRepository(_ context.Context, item CodeRepository) (CodeRepository, error) {
	r.items = append(r.items, item)
	return item, nil
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
func (p *providerStub) ClonePublic(_ context.Context, repositoryURL, path string) error {
	p.cloneURL, p.clonePath = repositoryURL, path
	return nil
}

func (p *providerStub) SyncPublic(_ context.Context, repositoryURL, path string) error {
	p.syncURL, p.syncPath = repositoryURL, path
	return nil
}

func (*providerStub) PreparePublicTaskCheckout(context.Context, string, string, string) error {
	return nil
}

func (providerStub) ConfigSchema() json.RawMessage { return json.RawMessage(`{}`) }

func (providerStub) ValidateConfig(json.RawMessage) error { return nil }

func (providerStub) ValidateAuthentication(context.Context, json.RawMessage, Credentials) error {
	return nil
}

func (providerStub) DiscoverRepositories(context.Context, json.RawMessage, Credentials, int, int) (RepositoryPage, error) {
	return RepositoryPage{}, nil
}

func (providerStub) ResolveRepository(_ context.Context, _ json.RawMessage, _ Credentials, id string) (RepositoryMetadata, error) {
	if id != "42" {
		return RepositoryMetadata{}, fmt.Errorf("repository unavailable")
	}
	return RepositoryMetadata{ID: id, Name: "acme/project", CloneURL: "https://github.com/acme/project.git", Private: true, DefaultBranch: "trunk"}, nil
}
func (providerStub) NormalizeRepositoryURL(value string) (string, error) { return value, nil }
func (providerStub) CloneAuthenticated(context.Context, RepositoryMetadata, Credentials, string) error {
	return nil
}

func (providerStub) SyncAuthenticated(context.Context, RepositoryMetadata, Credentials, string) error {
	return nil
}

func (providerStub) PrepareAuthenticatedTaskCheckout(context.Context, RepositoryMetadata, Credentials, string, string) error {
	return nil
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
	if err := service.Sync(context.Background(), "user", "org", "workspace", "repo"); err != nil {
		t.Fatalf("sync main: %v", err)
	}
	if provider.syncURL != repository.items[0].URL || provider.syncPath != expectedPath {
		t.Fatalf("unexpected sync arguments: url=%q path=%q", provider.syncURL, provider.syncPath)
	}
}

func TestCodingIntegrationCredentialsAreEncryptedWriteOnlyAndRotatable(t *testing.T) {
	repository := &repositoryStub{}
	box, err := secretbox.New("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(repository, workspaceStub{role: "admin"}, t.TempDir(), &providerStub{})
	service.SetSecretBox(box)
	withoutBox := NewService(&repositoryStub{}, workspaceStub{role: "admin"}, t.TempDir(), &providerStub{})
	if _, err := withoutBox.CreateIntegration(context.Background(), "user", "org", "workspace", CodingIntegrationInput{Name: "No key", ProviderName: "github", Credential: "token"}); !errors.Is(err, secretbox.ErrKeyUnavailable) {
		t.Fatalf("missing encryption key error=%v", err)
	}
	created, err := service.CreateIntegration(context.Background(), "user", "org", "workspace", CodingIntegrationInput{Name: "GitHub", ProviderName: "github", Credential: "first-token"})
	if err != nil {
		t.Fatal(err)
	}
	if repository.credentials[created.ID] == "first-token" {
		t.Fatal("credential stored as plaintext")
	}
	clear, err := box.Open([]byte(repository.credentials[created.ID]))
	if err != nil || clear != "first-token" {
		t.Fatalf("decrypt credential = %q, %v", clear, err)
	}
	listed, err := service.ListIntegrations(context.Background(), "user", "org", "workspace")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(listed)
	if string(encoded) == "" || string(encoded) == "first-token" || containsSecret(string(encoded), "first-token") {
		t.Fatalf("credential leaked in list: %s", encoded)
	}
	if err = service.UpdateIntegration(context.Background(), "user", "org", "workspace", created.ID, CodingIntegrationInput{Name: "GitHub rotated", Credential: "rotated-token"}); err != nil {
		t.Fatal(err)
	}
	credential, err := service.CodingCredential(context.Background(), "workspace", created.ID)
	if err != nil || credential.Token != "rotated-token" {
		t.Fatalf("resolved credential=%q err=%v", credential.Token, err)
	}
	if err = service.ValidateIntegration(context.Background(), "user", "org", "workspace", created.ID); err != nil {
		t.Fatal(err)
	}
}

func TestCodingIntegrationRolesAndRepositoryResolution(t *testing.T) {
	box, _ := secretbox.New("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	repository := &repositoryStub{}
	memberService := NewService(repository, workspaceStub{role: "member"}, t.TempDir(), &providerStub{})
	memberService.SetSecretBox(box)
	if _, err := memberService.CreateIntegration(context.Background(), "u", "o", "w", CodingIntegrationInput{Name: "No", ProviderName: "github", Credential: "token"}); !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatalf("member create integration error=%v", err)
	}
	service := NewService(repository, workspaceStub{role: "owner"}, t.TempDir(), &providerStub{})
	service.SetSecretBox(box)
	integration, err := service.CreateIntegration(context.Background(), "u", "o", "w", CodingIntegrationInput{Name: "GitHub", ProviderName: "github", Credential: "token"})
	if err != nil {
		t.Fatal(err)
	}
	viewer := NewService(repository, workspaceStub{role: "viewer"}, t.TempDir(), &providerStub{})
	viewer.SetSecretBox(box)
	if _, err = viewer.BrowseRepositories(context.Background(), "u", "o", "w", integration.ID, 1, 50); !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatalf("viewer repository browsing error=%v", err)
	}
	registered, err := service.RegisterRepository(context.Background(), "u", "o", "w", integration.ID, "42")
	if err != nil {
		t.Fatal(err)
	}
	if registered.URL != "https://github.com/acme/project.git" || registered.DefaultBranch != "trunk" || !registered.Private || registered.ProviderRepositoryID != "42" {
		t.Fatalf("registered metadata=%+v", registered)
	}
	if _, err = service.RegisterRepository(context.Background(), "u", "o", "w", integration.ID, "forged"); err == nil {
		t.Fatal("forged provider repository selection accepted")
	}
	if err = service.DeleteIntegration(context.Background(), "u", "o", "w", integration.ID); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatalf("delete referenced integration error=%v", err)
	}
	if _, _, err = repository.GetCodingIntegration(context.Background(), "other-workspace", integration.ID); !errors.Is(err, modelerrors.ErrNotFound) {
		t.Fatalf("cross-workspace integration read error=%v", err)
	}
}

func containsSecret(value, secret string) bool { return strings.Contains(value, secret) }
