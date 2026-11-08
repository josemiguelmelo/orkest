package orchestration

import (
	"context"
	"errors"
	"testing"

	codemodel "github.com/josemiguelmelo/orkest/internal/server/coderepository/model"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	taskmodel "github.com/josemiguelmelo/orkest/internal/server/task/model"
)

type workspaceCredentialStub struct {
	t            *testing.T
	integrations []codemodel.CodingIntegration
	err          error
	calls        int
}

func (s *workspaceCredentialStub) ListIntegrations(_ context.Context, userID, orgID, workspaceID string) ([]codemodel.CodingIntegration, error) {
	if userID != "user" || orgID != "org" || workspaceID != "workspace" {
		s.t.Fatal("integration lookup lost authorization scope")
	}
	return s.integrations, nil
}

func (s *workspaceCredentialStub) CodingCredential(_ context.Context, workspaceID, integrationID string) (codemodel.Credentials, error) {
	if workspaceID != "workspace" || integrationID != "integration" {
		s.t.Fatal("credential lookup lost workspace scope")
	}
	s.calls++
	return codemodel.Credentials{Token: "test-token"}, s.err
}

func TestUnlinkedTaskUsesExistingWorkspaceCredentialResolver(t *testing.T) {
	resolver := &workspaceCredentialStub{t: t, integrations: []codemodel.CodingIntegration{{ID: "integration", ProviderName: "custom-provider", BaseURL: "https://code.example/base/"}}}
	dispatcher := TaskDispatcher{codingCredentials: resolver}
	config := orchestrationmodel.RunConfig{}
	task := taskmodel.Task{WorkspaceID: "workspace"}
	if err := dispatcher.resolveTaskRepositories(context.Background(), "user", "org", task, &config); err != nil {
		t.Fatal(err)
	}
	if resolver.calls != 1 || len(config.Repositories) != 0 || len(config.GitCredentials) != 1 || !config.GitCredentialsRequired {
		t.Fatal("unlinked task did not receive integration credentials")
	}
	if got := config.GitCredentials[0]; got.RepositoryURL != "https://code.example/base/" || got.RepositoryPath != "" || got.Token != "test-token" {
		t.Fatal("incorrect workspace credential scope")
	}
	// A failed credential lookup must fail the run instead of silently losing auth.
	resolver.err = errors.New("credential unavailable")
	if err := dispatcher.resolveTaskRepositories(context.Background(), "user", "org", task, &config); !errors.Is(err, resolver.err) {
		t.Fatalf("credential failure not propagated: %v", err)
	}
	// Continued sessions retain their credential requirement when the integration disappears.
	resolver.integrations = nil
	config.GitCredentialsRequired = true
	if err := dispatcher.resolveTaskRepositories(context.Background(), "user", "org", task, &config); err != nil {
		t.Fatal(err)
	}
	if !config.GitCredentialsRequired || len(config.GitCredentials) != 0 {
		t.Fatal("continued session lost its missing-credential guard")
	}
}
