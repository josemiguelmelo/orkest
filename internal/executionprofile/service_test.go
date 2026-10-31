package executionprofile

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/core/model/errors"
	coreruntime "github.com/josemiguelmelo/orkest/internal/core/runtime"
	"github.com/josemiguelmelo/orkest/internal/workspace"
)

type memoryRepo struct {
	Repository
	profile  Profile
	build    Build
	claimed  bool
	finishes int
}

func (r *memoryRepo) Get(_ context.Context, ws, id string) (Profile, error) {
	if ws != r.profile.WorkspaceID || id != r.profile.ID {
		return Profile{}, modelerrors.ErrNotFound
	}
	return r.profile, nil
}

func (r *memoryRepo) Save(_ context.Context, p Profile, b Build, _ bool) (Profile, error) {
	r.profile = p
	r.build = b
	return p, nil
}

func (r *memoryRepo) Claim(_ context.Context, owner string) (Build, bool, error) {
	if r.claimed {
		return Build{}, false, nil
	}
	r.claimed = true
	r.build.Owner = owner
	return r.build, true, nil
}
func (*memoryRepo) Heartbeat(context.Context, string, string) error { return nil }
func (*memoryRepo) PinInputs(context.Context, Build) error          { return nil }
func (r *memoryRepo) Finish(_ context.Context, b Build, status, image, diagnostic string) error {
	if b.ID != r.build.ID || b.Owner != r.build.Owner {
		return errors.New("lost identity")
	}
	r.finishes++
	r.build.Status = status
	r.build.ExecutionImage = image
	r.build.BuildError = diagnostic
	return nil
}

type workspaceAccess struct{ role string }

func (a workspaceAccess) GetWorkspace(context.Context, string, string, string) (workspace.Workspace, error) {
	return workspace.Workspace{Role: a.role}, nil
}

type builderFake struct {
	builds, reconciles int
	fail               bool
}

func (*builderFake) ResolveInputs(_ context.Context, b Build) (Build, error) { return b, nil }
func (b *builderFake) BuildAndPush(context.Context, Build) (string, error) {
	b.builds++
	if b.fail {
		return "", errors.New("sensitive registry error")
	}
	return "registry/image@sha256:123", nil
}

func (b *builderFake) Reconcile(context.Context, Build) (string, error) {
	b.reconciles++
	return "registry/image@sha256:123", nil
}

func TestProfileAuthorizationAndScope(t *testing.T) {
	ctx := context.Background()
	r := &memoryRepo{profile: Profile{ID: "p", WorkspaceID: "w", Input: Input{Runtime: coreruntime.OpenCode}}}
	input := Input{Name: "Java", Runtime: coreruntime.OpenCode, ToolchainImage: "example.com/team/java:21"}
	for _, role := range []string{"viewer", "member"} {
		s := NewService(r, workspaceAccess{role}, "example.com/outputs", nil)
		if _, err := s.Save(ctx, "u", "o", "w", "", 0, input); !errors.Is(err, modelerrors.ErrForbidden) {
			t.Fatalf("%s allowed build: %v", role, err)
		}
	}
	s := NewService(r, workspaceAccess{"admin"}, "example.com/outputs", nil)
	if _, err := s.Resolve(ctx, "other", "p", coreruntime.OpenCode); !errors.Is(err, modelerrors.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.Resolve(ctx, "w", "p", coreruntime.OpenCode); !errors.Is(err, modelerrors.ErrConflict) {
		t.Fatal("unprepared profile allowed", err)
	}
	if _, err := s.Save(ctx, "u", "o", "w", "", 0, input); err != nil {
		t.Fatal(err)
	}
	if r.build.ID == "" || r.build.BaseImage == "" || r.build.RuntimeVersion == "" {
		t.Fatal("missing captured build inputs")
	}
}

func TestWorkerReconcilesExpiredAttemptWithoutRebuilding(t *testing.T) {
	r := &memoryRepo{build: Build{ID: "b", Status: "reconciling"}}
	builder := &builderFake{}
	w := NewWorker(r, builder, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := w.process(context.Background()); err != nil {
		t.Fatal(err)
	}
	if builder.builds != 0 || builder.reconciles != 1 || r.build.Status != "ready" {
		t.Fatalf("unexpected recovery: %+v %+v", builder, r.build)
	}
}

func TestWorkerFailureIsSanitizedAndNotRetried(t *testing.T) {
	r := &memoryRepo{build: Build{ID: "b", Status: "building"}}
	builder := &builderFake{fail: true}
	w := NewWorker(r, builder, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := w.process(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := w.process(context.Background()); err != nil {
		t.Fatal(err)
	}
	if builder.builds != 1 || r.build.Status != "failed" || r.build.BuildError == "sensitive registry error" {
		t.Fatal("unsafe failure handling", r.build)
	}
}
