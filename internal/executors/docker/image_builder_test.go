package docker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/executionprofile"
)

func TestDockerBuilderPublishesAndReconciles(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "commands")
	t.Setenv("ORKEST_TEST_COMMANDS", log)
	hash := "sha256:" + strings.Repeat("a", 64)
	script := `#!/bin/sh
set -eu
printf '%s\n' "$1" >> "$ORKEST_TEST_COMMANDS"
if [ "$1" = 'image' ]; then
 printf '%s' '[{"RepoDigests":["example.com/output@` + hash + `"],"Config":{"Labels":{"dev.orkest.build":"attempt"}}}]'
fi
`
	binary := filepath.Join(dir, "docker")
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	store := &ImageStore{binary: binary, user: "1000:1000"}
	builder, err := NewImageBuilder(store)
	if err != nil {
		t.Fatal(err)
	}
	build := executionprofile.Build{ID: "attempt", Runtime: "opencode", RuntimeVersion: "opencode v2.1.4", ResolvedBase: "example.com/base@" + hash, ResolvedToolchain: "example.com/tools@" + hash, Destination: "example.com/output:build-attempt"}
	artifact, err := builder.BuildAndPush(context.Background(), build)
	if err != nil {
		t.Fatal(err)
	}
	if artifact != "example.com/output@"+hash {
		t.Fatal("incorrect artifact", artifact)
	}
	commands, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if string(commands) != "build\nrun\nrm\npush\npull\nimage\nrun\nrm\n" {
		t.Fatal("unexpected build/publication sequence", string(commands))
	}
	if err = os.WriteFile(log, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = builder.Reconcile(context.Background(), build); err != nil {
		t.Fatal(err)
	}
	commands, err = os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(commands), "build") || strings.Contains(string(commands), "push") {
		t.Fatal("reconciliation rebuilt or republished")
	}
}

func TestDockerBuilderDoesNotPushFailedValidation(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "commands")
	t.Setenv("ORKEST_TEST_COMMANDS", log)
	binary := filepath.Join(dir, "docker")
	script := `#!/bin/sh
printf '%s\n' "$1" >> "$ORKEST_TEST_COMMANDS"
if [ "$1" = 'run' ]; then
 echo private-token >&2
 exit 1
fi
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	builder, err := NewImageBuilder(&ImageStore{binary: binary, user: "1000:1000"})
	if err != nil {
		t.Fatal(err)
	}
	hash := "sha256:" + strings.Repeat("a", 64)
	build := executionprofile.Build{ID: "attempt", Runtime: "opencode", RuntimeVersion: "opencode v2.1.4", ResolvedBase: "example.com/base@" + hash, ResolvedToolchain: "example.com/tools@" + hash, Destination: "example.com/output:build-attempt"}
	if _, err = builder.BuildAndPush(context.Background(), build); err == nil || strings.Contains(err.Error(), "private-token") {
		t.Fatal("invalid validation failure handling", err)
	}
	commands, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(commands), "push") {
		t.Fatal("published invalid artifact")
	}
}
