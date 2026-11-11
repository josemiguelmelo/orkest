package mcpserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/platform/secretbox"
	mcpservermodel "github.com/josemiguelmelo/orkest/internal/server/mcpserver/model"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
)

type repositoryStub struct {
	server mcpservermodel.Server
	secret string
}

func (r *repositoryStub) Create(_ context.Context, server mcpservermodel.Server, secret string) (mcpservermodel.Server, error) {
	server.HasHeaders = server.Transport != "stdio" && secret != ""
	server.HasEnvironment = server.Transport == "stdio" && secret != ""
	r.server, r.secret = server, secret
	return server, nil
}

func (r *repositoryStub) Update(ctx context.Context, server mcpservermodel.Server, secret string) (mcpservermodel.Server, error) {
	return r.Create(ctx, server, secret)
}

func (r *repositoryStub) Delete(context.Context, string, string) error { return nil }

func (r *repositoryStub) Get(context.Context, string, string) (mcpservermodel.Server, string, error) {
	return r.server, r.secret, nil
}

func (r *repositoryStub) List(context.Context, string) ([]mcpservermodel.Server, error) {
	return []mcpservermodel.Server{r.server}, nil
}

type workspaceStub struct{ role string }

func (w workspaceStub) GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error) {
	return workspacemodel.Workspace{Role: w.role}, nil
}

func TestCreateEncryptsHeadersAndResolveDecryptsThem(t *testing.T) {
	box, err := secretbox.New("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	repository := &repositoryStub{}
	service := NewMCPServerService(repository, box, workspaceStub{role: "member"})
	created, err := service.Create(context.Background(), "user", "org", "workspace", mcpservermodel.Input{Name: "Remote", URL: "https://example.test/mcp", Headers: map[string]string{"Authorization": "Bearer private"}, Transport: "http"})
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
	repository := &repositoryStub{server: mcpservermodel.Server{ID: "server", WorkspaceID: "workspace", Name: "Remote", URL: "https://example.test/mcp", Transport: "http"}}
	sealed, _ := box.Seal(`{"Authorization":"Bearer private"}`)
	repository.secret = string(sealed)
	service := NewMCPServerService(repository, box, workspaceStub{role: "admin"})
	if _, err := service.Update(context.Background(), "user", "org", "workspace", "server", mcpservermodel.Input{Transport: "http"}); err != nil {
		t.Fatal(err)
	}
	resolved, err := service.Resolve(context.Background(), "workspace", []string{"server"})
	if err != nil || resolved[0].Headers["Authorization"] != "Bearer private" {
		t.Fatalf("omitted headers were not preserved: %#v %v", resolved, err)
	}
	for _, in := range []mcpservermodel.Input{
		{Name: "bad", URL: "file:///etc/passwd", Transport: "http"},
		{Name: "bad", URL: "https://example.test/mcp", Headers: map[string]string{"Bad Header": "value"}, Transport: "http"},
		{Name: "bad", URL: "https://example.test/mcp", Headers: map[string]string{"X-Test": "one\r\ntwo"}, Transport: "http"},
	} {
		if _, err := service.Create(context.Background(), "user", "org", "workspace", in); err == nil {
			t.Fatal("invalid MCP configuration accepted")
		}
	}
}

func TestMutationRequiresWorkspacePermissionAndEncryptionKey(t *testing.T) {
	input := mcpservermodel.Input{Name: "Remote", URL: "https://example.test/mcp", Transport: "http"}
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

func TestStdioCredentialsAndUpdates(t *testing.T) {
	box, _ := secretbox.New("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	repo := &repositoryStub{}
	service := NewMCPServerService(repo, box, workspaceStub{role: "member"})
	ctx := context.Background()
	created, err := service.Create(ctx, "user", "org", "workspace", mcpservermodel.Input{Name: "Tools", Transport: "stdio", Command: "/opt/mcp/server", Args: []string{"literal space", "$(no expansion)"}, Environment: map[string]string{"API_TOKEN": "private-token"}})
	if err != nil {
		t.Fatal(err)
	}
	if !created.HasEnvironment || created.HasHeaders || strings.Contains(repo.secret, "private-token") {
		t.Fatal("stdio credentials were not encrypted separately")
	}
	if _, err := service.Update(ctx, "user", "org", "workspace", created.ID, mcpservermodel.Input{Name: "Renamed", Transport: "stdio"}); err != nil {
		t.Fatal(err)
	}
	resolved, err := service.Resolve(ctx, "workspace", []string{created.ID})
	if err != nil || len(resolved) != 1 || resolved[0].Environment["API_TOKEN"] != "private-token" || resolved[0].Command != "/opt/mcp/server" || len(resolved[0].Args) != 2 {
		t.Fatal("update lost stdio configuration", err)
	}
	if _, err := service.Update(ctx, "user", "org", "workspace", created.ID, mcpservermodel.Input{Transport: "http", URL: "https://example.test"}); err == nil {
		t.Fatal("transport changed")
	}
	if _, err := service.Update(ctx, "user", "org", "workspace", created.ID, mcpservermodel.Input{Transport: "stdio", Args: []string{}, Environment: map[string]string{}}); err != nil {
		t.Fatal(err)
	}
	if repo.secret != "" || len(repo.server.Args) != 0 || repo.server.HasEnvironment {
		t.Fatal("empty values did not clear credentials and arguments")
	}
}

func TestRejectsInvalidStdioConfiguration(t *testing.T) {
	service := NewMCPServerService(&repositoryStub{}, nil, workspaceStub{role: "member"})
	for _, input := range []mcpservermodel.Input{
		{Name: "Tools", Transport: "pipe", Command: "/server"},
		{Name: "Tools", Transport: "stdio", Command: "server"},
		{Name: "Tools", Transport: "stdio", Command: "/server", URL: "https://example.test"},
		{Name: "Tools", Transport: "stdio", Command: "/server", Headers: map[string]string{"Token": "value"}},
		{Name: "Tools", Transport: "stdio", Command: "/server", Args: []string{"bad\x00argument"}},
		{Name: "Tools", Transport: "stdio", Command: "/server", Environment: map[string]string{"BAD-NAME": "value"}},
		{Name: "Tools", URL: "https://example.test", Command: "/server", Transport: "http"},
		{Name: "Tools", URL: "https://example.test#fragment", Transport: "http"},
	} {
		if _, err := service.Create(context.Background(), "user", "org", "workspace", input); err == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
	input := mcpservermodel.Input{Name: "Tools", Transport: "stdio", Command: "/server", Environment: map[string]string{"TOKEN": "value"}}
	if _, err := service.Create(context.Background(), "user", "org", "workspace", input); !errors.Is(err, secretbox.ErrKeyUnavailable) {
		t.Fatal("missing encryption key accepted", err)
	}
	if _, err := NewMCPServerService(&repositoryStub{}, nil, workspaceStub{role: "viewer"}).Create(context.Background(), "user", "org", "workspace", input); err == nil {
		t.Fatal("viewer registered an executable")
	}
}

func TestMissingTransportIsRejected(t *testing.T) {
	repo := &repositoryStub{server: mcpservermodel.Server{ID: "server", Name: "Tools", Transport: "http", URL: "https://example.test"}}
	service := NewMCPServerService(repo, nil, workspaceStub{role: "member"})
	input := mcpservermodel.Input{Name: "Tools", URL: "https://example.test"}
	if _, err := service.Create(context.Background(), "user", "org", "workspace", input); err == nil {
		t.Fatal("missing transport accepted on create")
	}
	if _, err := service.Update(context.Background(), "user", "org", "workspace", "server", input); err == nil {
		t.Fatal("missing transport accepted on update")
	}
}
