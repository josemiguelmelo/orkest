package workspaceenv

import (
	"context"
	"errors"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	"github.com/josemiguelmelo/orkest/internal/platform/secretbox"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
	workspaceenvmodel "github.com/josemiguelmelo/orkest/internal/server/workspaceenv/model"
)

type repositoryStub struct {
	values map[string]workspaceenvmodel.StoredValue
}

func (r *repositoryStub) List(context.Context, string) ([]workspaceenvmodel.StoredValue, error) {
	values := make([]workspaceenvmodel.StoredValue, 0, len(r.values))
	for _, value := range r.values {
		values = append(values, value)
	}
	return values, nil
}

func (r *repositoryStub) Resolve(ctx context.Context, workspaceID string) ([]workspaceenvmodel.StoredValue, error) {
	return r.List(ctx, workspaceID)
}

func (r *repositoryStub) Put(_ context.Context, _, name, value string, secret bool) error {
	r.values[name] = workspaceenvmodel.StoredValue{Name: name, Value: value, Encrypted: secret}
	return nil
}

func (r *repositoryStub) Delete(_ context.Context, _, name string) error {
	if _, ok := r.values[name]; !ok {
		return modelerrors.ErrNotFound
	}
	delete(r.values, name)
	return nil
}

type workspaceStub struct{ role string }

func (w workspaceStub) GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error) {
	return workspacemodel.Workspace{Role: w.role}, nil
}

func TestSecretsAreWriteOnlyAndResolveForRuns(t *testing.T) {
	box, err := secretbox.New("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	repo := &repositoryStub{values: map[string]workspaceenvmodel.StoredValue{}}
	service := NewWorkspaceEnvironmentService(repo, workspaceStub{role: "admin"}, box)
	if err := service.Put(context.Background(), "user", "org", "workspace", "API_TOKEN", workspaceenvmodel.Input{Value: "top-secret", Secret: true}); err != nil {
		t.Fatal(err)
	}
	if repo.values["API_TOKEN"].Value == "top-secret" {
		t.Fatal("secret was stored in plaintext")
	}
	listed, err := service.List(context.Background(), "user", "org", "workspace")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || !listed[0].Secret || listed[0].Value != "" {
		t.Fatalf("unexpected API value: %#v", listed)
	}
	resolved, err := service.Resolve(context.Background(), "workspace")
	if err != nil || resolved["API_TOKEN"] != "top-secret" {
		t.Fatalf("resolved=%v err=%v", resolved, err)
	}
}

func TestManageRequiresAdminAndValidNames(t *testing.T) {
	repo := &repositoryStub{values: map[string]workspaceenvmodel.StoredValue{}}
	member := NewWorkspaceEnvironmentService(repo, workspaceStub{role: "member"}, nil)
	if err := member.Put(context.Background(), "user", "org", "workspace", "VALUE", workspaceenvmodel.Input{Value: "x"}); !errors.Is(err, modelerrors.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
	admin := NewWorkspaceEnvironmentService(repo, workspaceStub{role: "admin"}, nil)
	for _, name := range []string{"1INVALID", "ORKEST_TOKEN", "OPENCODE_CONFIG"} {
		if err := admin.Put(context.Background(), "user", "org", "workspace", name, workspaceenvmodel.Input{Value: "x"}); !errors.Is(err, modelerrors.ErrInvalid) {
			t.Errorf("%s: expected invalid, got %v", name, err)
		}
	}
}

func TestEnvironmentSnapshotIsEncrypted(t *testing.T) {
	box, err := secretbox.New("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	service := NewWorkspaceEnvironmentService(nil, nil, box)
	sealed, err := service.SealSnapshot(workspaceenvmodel.RuntimeValues{"TOKEN": "sensitive"})
	if err != nil {
		t.Fatal(err)
	}
	if sealed == "" || sealed == `{"TOKEN":"sensitive"}` {
		t.Fatalf("snapshot was not encrypted: %q", sealed)
	}
	opened, err := service.OpenSnapshot(sealed)
	if err != nil || opened["TOKEN"] != "sensitive" {
		t.Fatalf("snapshot=%v err=%v", opened, err)
	}
}
