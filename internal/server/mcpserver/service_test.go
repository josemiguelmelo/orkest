package mcpserver

import (
	"context"
	"errors"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/platform/secretbox"
	"github.com/josemiguelmelo/orkest/internal/server/workspace"
)

type repositoryStub struct {
	server Server
	secret string
}

func (r *repositoryStub) Create(_ context.Context, id, workspaceID, name, endpoint, secret string) (Server, error) {
	r.server = Server{ID: id, WorkspaceID: workspaceID, Name: name, URL: endpoint, HasHeaders: secret != ""}
	r.secret = secret
	return r.server, nil
}

func (r *repositoryStub) Update(_ context.Context, workspaceID, id, name, endpoint, secret string) (Server, error) {
	r.server = Server{ID: id, WorkspaceID: workspaceID, Name: name, URL: endpoint, HasHeaders: secret != ""}
	r.secret = secret
	return r.server, nil
}
func (r *repositoryStub) Delete(context.Context, string, string) error { return nil }
func (r *repositoryStub) Get(context.Context, string, string) (Server, string, error) {
	return r.server, r.secret, nil
}

func (r *repositoryStub) List(context.Context, string) ([]Server, error) {
	return []Server{r.server}, nil
}

type workspaceStub struct{ role string }

func (w workspaceStub) GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error) {
	return workspace.Workspace{Role: w.role}, nil
}

func TestCreateEncryptsHeadersAndResolveDecryptsThem(t *testing.T) {
	box, err := secretbox.New("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	repository := &repositoryStub{}
	service := NewMCPServerService(repository, box, workspaceStub{role: "member"})
	created, err := service.Create(context.Background(), "user", "org", "workspace", Input{Name: "Remote", URL: "https://example.test/mcp", Headers: map[string]string{"Authorization": "Bearer private"}})
	if err != nil {
		t.Fatal(err)
	}
	if !created.HasHeaders || repository.secret == "" || repository.secret == `{"Authorization":"Bearer private"}` {
		t.Fatal("headers were not encrypted")
	}
	resolved, err := service.Resolve(context.Background(), "workspace", []string{created.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 || resolved[0].Headers["Authorization"] != "Bearer private" {
		t.Fatalf("unexpected resolved headers: %#v", resolved)
	}
}

func TestUpdatePreservesOmittedHeadersAndRejectsUnsafeValues(t *testing.T) {
	box, _ := secretbox.New("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	repository := &repositoryStub{server: Server{ID: "server", WorkspaceID: "workspace", Name: "Remote", URL: "https://example.test/mcp"}}
	sealed, _ := box.Seal(`{"Authorization":"Bearer private"}`)
	repository.secret = string(sealed)
	service := NewMCPServerService(repository, box, workspaceStub{role: "admin"})
	if _, err := service.Update(context.Background(), "user", "org", "workspace", "server", Input{}); err != nil {
		t.Fatal(err)
	}
	resolved, err := service.Resolve(context.Background(), "workspace", []string{"server"})
	if err != nil || resolved[0].Headers["Authorization"] != "Bearer private" {
		t.Fatalf("omitted headers were not preserved: %#v %v", resolved, err)
	}
	for _, in := range []Input{
		{Name: "bad", URL: "file:///etc/passwd"},
		{Name: "bad", URL: "https://example.test/mcp", Headers: map[string]string{"Bad Header": "value"}},
		{Name: "bad", URL: "https://example.test/mcp", Headers: map[string]string{"X-Test": "one\r\ntwo"}},
	} {
		if _, err := service.Create(context.Background(), "user", "org", "workspace", in); err == nil {
			t.Fatal("invalid MCP configuration accepted")
		}
	}
}

func TestMutationRequiresWorkspacePermissionAndEncryptionKey(t *testing.T) {
	input := Input{Name: "Remote", URL: "https://example.test/mcp"}
	if _, err := NewMCPServerService(&repositoryStub{}, nil, workspaceStub{role: "viewer"}).Create(context.Background(), "user", "org", "workspace", input); err == nil {
		t.Fatal("viewer mutation accepted")
	}
	service := NewMCPServerService(&repositoryStub{}, nil, workspaceStub{role: "member"})
	if _, err := service.Create(context.Background(), "user", "org", "workspace", input); err != nil {
		t.Fatalf("URL-only MCP server should not require encryption: %v", err)
	}
	input.Headers = map[string]string{"Authorization": "Bearer private"}
	if _, err := service.Create(context.Background(), "user", "org", "workspace", input); !errors.Is(err, secretbox.ErrKeyUnavailable) {
		t.Fatalf("unexpected key error: %v", err)
	}
}
