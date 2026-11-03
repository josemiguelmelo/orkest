package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/artifact"
	"github.com/josemiguelmelo/orkest/internal/coderepository"
	"github.com/josemiguelmelo/orkest/internal/task"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type artifactTestServices struct {
	artifact.Repository
	scope artifact.Scope
}

func (*artifactTestServices) Get(context.Context, string, string, string, string) (task.Task, error) {
	return task.Task{}, nil
}

func (*artifactTestServices) List(context.Context, string, string, string) ([]coderepository.CodeRepository, error) {
	return nil, nil
}

func (*artifactTestServices) AuthorizeExecution(context.Context, string, string, string, string, bool) error {
	return nil
}

type artifactTestRepository struct {
	artifact.Repository
	services *artifactTestServices
}

func (r artifactTestRepository) Publish(_ context.Context, scope artifact.Scope, _ artifact.PublishInput) (artifact.Revision, error) {
	r.services.scope = scope
	return artifact.Revision{ID: "revision"}, nil
}

type bearerTransport struct{ token string }

func (t bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", t.token)
	return http.DefaultTransport.RoundTrip(r)
}

func TestArtifactMCPPublishingUsesAuthenticatedRun(t *testing.T) {
	services := &artifactTestServices{}
	tools, err := NewAgentTools(strings.Repeat("k", 32), "https://example.test", services, services)
	if err != nil {
		t.Fatal(err)
	}
	tools.SetArtifacts(artifact.NewService(artifactTestRepository{services: services}, services))
	runtime, err := tools.RuntimeServerForRun("user", "org", "workspace", "task", "run")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(tools.Handler())
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL, HTTPClient: &http.Client{Transport: bearerTransport{token: runtime.Headers["Authorization"]}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "publish_artifact", Arguments: map[string]any{"kind": "plan", "title": "Plan", "expectedRevision": 0, "idempotencyKey": "once", "content": map[string]any{"mediaType": "text/markdown", "text": "Plan"}}})
	if err != nil || result.IsError {
		t.Fatalf("publish: %+v %v", result, err)
	}
	if services.scope != (artifact.Scope{UserID: "user", OrganizationID: "org", WorkspaceID: "workspace", TaskID: "task", RunID: "run"}) {
		t.Fatalf("unbound scope: %+v", services.scope)
	}
}
