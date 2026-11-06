package buildkit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/server/executionprofile"
)

type storeFake struct {
	verified []string
	resolves int
}

func (s *storeFake) ResolveInputs(_ context.Context, b executionprofile.Build) (executionprofile.Build, error) {
	s.resolves++
	return b, nil
}

func (s *storeFake) Verify(_ context.Context, _ executionprofile.Build, ref string) (string, error) {
	s.verified = append(s.verified, ref)
	return ref, nil
}

func buildFixture() executionprofile.Build {
	hash := "sha256:" + strings.Repeat("a", 64)
	return executionprofile.Build{ID: "attempt", Runtime: "opencode", RuntimeVersion: "opencode v2.1.4", ResolvedBase: "example.com/base@" + hash, ResolvedToolchain: "example.com/tools@" + hash, Destination: "example.com/output:build-attempt"}
}

func buildctlScript(t *testing.T, script string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "buildctl")
	if err := os.WriteFile(p, []byte("#!/bin/sh\nset -eu\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBuildKitPublishesAndValidatesExactExporterDigest(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("BUILDKIT_TEST_ARGS", argsFile)
	hash := "sha256:" + strings.Repeat("b", 64)
	binary := buildctlScript(t, `printf '%s\n' "$@" > "$BUILDKIT_TEST_ARGS"
while [ "$#" -gt 0 ]; do
 if [ "$1" = '--metadata-file' ]; then
  shift
  printf '%s' '{"containerimage.digest":"`+hash+`"}' > "$1"
 fi
 shift
done
`)
	store := &storeFake{}
	b, err := New(Config{Binary: binary, Address: "unix:///run/buildkit/test.sock", TLSCA: "ca.pem", TLSCert: "client.pem", TLSKey: "key.pem"}, store)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := b.BuildAndPush(context.Background(), buildFixture())
	if err != nil {
		t.Fatal(err)
	}
	if ref != "example.com/output@"+hash || len(store.verified) != 1 || store.verified[0] != ref {
		t.Fatal("did not validate exact exported artifact", ref, store.verified)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--addr\nunix:///run/buildkit/test.sock", "--tlscacert\nca.pem", "--tlskey\nkey.pem", "--frontend\ndockerfile.v0", "--output\ntype=image,name=example.com/output:build-attempt,push=true", "label:dev.orkest.build=attempt", "build-arg:TOOLCHAIN=" + buildFixture().ResolvedToolchain} {
		if !strings.Contains(string(args), want) {
			t.Fatal("missing BuildKit argument", want)
		}
	}
}

func TestBuildKitRejectsUnpinnedInputs(t *testing.T) {
	store := &storeFake{}
	b, err := New(Config{Binary: buildctlScript(t, "exit 99\n")}, store)
	if err != nil {
		t.Fatal(err)
	}
	fixture := buildFixture()
	fixture.ResolvedToolchain = "local-tools:latest"
	if _, err = b.BuildAndPush(context.Background(), fixture); err == nil || len(store.verified) != 0 {
		t.Fatal("accepted unpinned input")
	}
}

func TestBuildKitFailureDoesNotExposeBuildOutput(t *testing.T) {
	store := &storeFake{}
	b, err := New(Config{Binary: buildctlScript(t, "echo private-token >&2\nexit 1\n")}, store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = b.BuildAndPush(context.Background(), buildFixture()); err == nil || strings.Contains(err.Error(), "private-token") || len(store.verified) != 0 {
		t.Fatal("invalid failure handling", err)
	}
}

func TestBuildKitCancelsBuildClient(t *testing.T) {
	b, err := New(Config{Binary: buildctlScript(t, "exec sleep 30\n")}, &storeFake{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err = b.BuildAndPush(ctx, buildFixture()); err == nil || !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatal("build did not cancel", err)
	}
}

func TestReconcileDoesNotBuild(t *testing.T) {
	store := &storeFake{}
	b, err := New(Config{Binary: buildctlScript(t, "exit 99\n")}, store)
	if err != nil {
		t.Fatal(err)
	}
	fixture := buildFixture()
	ref, err := b.Reconcile(context.Background(), fixture)
	if err != nil || ref != fixture.Destination || len(store.verified) != 1 {
		t.Fatal("incorrect reconciliation", ref, err)
	}
}

func TestBuildKitRejectsMissingOrInvalidExporterDigest(t *testing.T) {
	for _, metadata := range []string{`{}`, `{"containerimage.digest":"not-a-digest"}`, `invalid-json`} {
		t.Run(metadata, func(t *testing.T) {
			binary := buildctlScript(t, `while [ "$#" -gt 0 ]; do
 if [ "$1" = '--metadata-file' ]; then
  shift
  printf '%s' '`+metadata+`' > "$1"
 fi
 shift
done
`)
			store := &storeFake{}
			b, err := New(Config{Binary: binary}, store)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = b.BuildAndPush(context.Background(), buildFixture()); err == nil || len(store.verified) != 0 {
				t.Fatal("invalid export was validated", err)
			}
		})
	}
}
