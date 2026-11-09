package workspacemount

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"testing"

	modelerrors "github.com/josemiguelmelo/orkest/internal/contracts/problem"
	workspacemodel "github.com/josemiguelmelo/orkest/internal/server/workspace/model"
	mountmodel "github.com/josemiguelmelo/orkest/internal/server/workspacemount/model"
)

type repositoryStub struct {
	values map[string][]mountmodel.Mount
	err    error
}

func (r *repositoryStub) List(_ context.Context, workspaceID string) ([]mountmodel.Mount, error) {
	return slices.Clone(r.values[workspaceID]), r.err
}

func (r *repositoryStub) Put(_ context.Context, workspaceID string, mounts []mountmodel.Mount) error {
	r.values[workspaceID] = slices.Clone(mounts)
	return r.err
}

type accessStub struct {
	role string
	err  error
}

func (a accessStub) GetWorkspace(context.Context, string, string, string) (workspacemodel.Workspace, error) {
	return workspacemodel.Workspace{Role: a.role}, a.err
}

func TestWorkspaceMountsAuthorizationAndResolution(t *testing.T) {
	ctx := context.Background()
	for _, role := range []string{"viewer", "member", "admin", "owner"} {
		t.Run(role, func(t *testing.T) {
			repo := &repositoryStub{values: make(map[string][]mountmodel.Mount)}
			service := New(repo, accessStub{role: role}, true)
			mounts := []mountmodel.Mount{{Source: "/srv/input", Target: "/mnt/host/input", ReadOnly: true}, {Source: "/srv/output", Target: "/mnt/host/output"}}
			err := service.Put(ctx, "user", "org", "workspace", mounts)
			if role == "viewer" || role == "member" {
				if !errors.Is(err, modelerrors.ErrForbidden) || len(repo.values) != 0 {
					t.Fatalf("non-admin mutation: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			settings, err := service.Get(ctx, "user", "org", "workspace")
			if err != nil || !settings.Enabled || !reflect.DeepEqual(settings.Mounts, mounts) {
				t.Fatalf("settings=%+v err=%v", settings, err)
			}
			resolved, err := service.Resolve(ctx, "workspace")
			if err != nil || !reflect.DeepEqual(resolved, mounts) {
				t.Fatalf("resolved=%+v err=%v", resolved, err)
			}
			other, err := service.Resolve(ctx, "other-workspace")
			if err != nil || len(other) != 0 {
				t.Fatal("selections crossed workspace boundary", err)
			}
			if err := service.Put(ctx, "user", "org", "workspace", nil); err != nil || len(repo.values["workspace"]) != 0 {
				t.Fatal("clearing mounts failed", err)
			}
		})
	}
}

func TestMountBoundariesWithoutEnvironmentConfiguration(t *testing.T) {
	service := New(nil, nil, true)
	for _, test := range []struct {
		name, source, target string
		readOnly             bool
		want                 error
	}{
		{"direct source", "/srv/custom", "/mnt/host/file", true, nil},
		{"direct write", "/srv/custom", "/mnt/host/output", false, nil},
		{"aws home", "/srv/aws", "~/.aws", true, nil},
		{"maven settings", "/srv/settings.xml", "~/.m2/settings.xml", true, nil},
		{"gradle properties", "/srv/gradle.properties", "~/.gradle/gradle.properties", true, nil},
		{"other home file", "/srv/netrc", "~/.netrc", true, nil},
		{"whole home", "/srv/home", "~/", true, modelerrors.ErrInvalid},
		{"bare home", "/srv/home", "~", true, modelerrors.ErrInvalid},
		{"named user home", "/srv/home", "~user/.aws", true, modelerrors.ErrInvalid},
		{"home traversal", "/srv/home", "~/../session.json", true, modelerrors.ErrInvalid},
		{"home dot", "/srv/home", "~/.aws/../.m2", true, modelerrors.ErrInvalid},
		{"home slash", "/srv/home", "~//.aws", true, modelerrors.ErrInvalid},
		{"home trailing slash", "/srv/home", "~/.aws/", true, modelerrors.ErrInvalid},
		{"root user home", "/srv/home", "/root/.aws", true, modelerrors.ErrInvalid},
		{"relative source", "srv/input", "/mnt/host/input", true, modelerrors.ErrInvalid},
		{"dot source", "/srv/input/../input", "/mnt/host/input", true, modelerrors.ErrInvalid},
		{"root target", "/srv/input", "/", true, modelerrors.ErrInvalid},
		{"work target", "/srv/input", "/workspace/input", true, modelerrors.ErrInvalid},
		{"state target", "/srv/input", "/agentctl-state", true, modelerrors.ErrInvalid},
		{"parent target", "/srv/input", "/mnt/host/../etc", true, modelerrors.ErrInvalid},
		{"base target", "/srv/input", "/mnt/host", true, modelerrors.ErrInvalid},
		{"nul target", "/srv/input", "/mnt/host/a\x00", true, modelerrors.ErrInvalid},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := service.validate([]mountmodel.Mount{{Source: test.source, Target: test.target, ReadOnly: test.readOnly}})
			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
	for _, target := range []string{"/mnt/host/input", "/mnt/host/input/nested"} {
		err := service.validate([]mountmodel.Mount{
			{Source: "/srv/input", Target: "/mnt/host/input", ReadOnly: true},
			{Source: "/srv/output", Target: target, ReadOnly: true},
		})
		if !errors.Is(err, modelerrors.ErrInvalid) {
			t.Fatal("overlapping targets accepted", err)
		}
		for _, targets := range [][2]string{{"~/.aws", "~/.aws/config"}, {"~/.aws/config", "~/.aws"}, {"~/.m2/settings.xml", "~/.m2/settings.xml"}} {
			err := service.validate([]mountmodel.Mount{
				{Source: "/srv/input", Target: targets[0], ReadOnly: true},
				{Source: "/srv/output", Target: targets[1], ReadOnly: true},
			})
			if !errors.Is(err, modelerrors.ErrInvalid) {
				t.Fatal("overlapping home targets accepted", err)
			}
		}
	}
	mounts := make([]mountmodel.Mount, 32)
	for i := range mounts {
		mounts[i] = mountmodel.Mount{Source: "/srv/input", Target: "/mnt/host/input" + strconv.Itoa(i), ReadOnly: true}
	}
	if err := service.validate(mounts); err != nil {
		t.Fatal("32 valid mounts were rejected", err)
	}
	mounts = append(mounts, mountmodel.Mount{Source: "/srv/input", Target: "/mnt/host/extra", ReadOnly: true})
	if err := service.validate(mounts); !errors.Is(err, modelerrors.ErrInvalid) {
		t.Fatal("mount count limit not enforced", err)
	}
	saved := []mountmodel.Mount{{Source: "/srv/input", Target: "/mnt/host/input", ReadOnly: true}}
	repo := &repositoryStub{values: map[string][]mountmodel.Mount{"workspace": saved}}
	disabled := New(repo, nil, false)
	if _, err := disabled.Resolve(context.Background(), "workspace"); !errors.Is(err, modelerrors.ErrInvalid) {
		t.Fatal("non-Docker executor accepted host mounts", err)
	}
}

func TestMountAccessAndStorageErrorsPropagate(t *testing.T) {
	ctx := context.Background()
	cause := errors.New("access denied")
	service := New(nil, accessStub{err: cause}, true)
	if _, err := service.Get(ctx, "user", "org", "workspace"); !errors.Is(err, cause) {
		t.Fatal(err)
	}
	if err := service.Put(ctx, "user", "org", "workspace", nil); !errors.Is(err, cause) {
		t.Fatal(err)
	}
	service.repository = &repositoryStub{err: cause}
	if _, err := service.Resolve(ctx, "workspace"); !errors.Is(err, cause) {
		t.Fatal(err)
	}
}
