package docker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	executionprofilemodel "github.com/josemiguelmelo/orkest/internal/server/executionprofile/model"
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
	build := executionprofilemodel.Build{ID: "attempt", Runtime: "opencode", RuntimeVersion: "opencode v2.1.4", ResolvedBase: "example.com/base@" + hash, ResolvedToolchain: "example.com/tools@" + hash, Destination: "example.com/output:build-attempt"}
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

func TestRuntimeProbeMatchesPinnedVersions(t *testing.T) {
	tests := []struct {
		name    string
		runtime coreruntime.Runtime
		version string
		want    string
	}{
		{name: "OpenCode", runtime: coreruntime.OpenCode, version: "opencode v2.1.4", want: `test "$(opencode --version)" = 'opencode v2.1.4'`},
		{name: "Claude Code", runtime: coreruntime.ClaudeCode, version: "0.99.0", want: `test "$(claude-agent-acp --version)" = '0.99.0'`},
		{name: "Codex", runtime: coreruntime.Codex, version: "2.3.0", want: `test "$(codex-acp --version)" = '@agentclientprotocol/codex-acp 2.3.0'`},
		{name: "GitHub Copilot", runtime: coreruntime.GitHubCopilot, version: "1.0.105", want: `test "$(copilot --version | head -n 1)" = 'GitHub Copilot CLI 1.0.105.'`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := runtimeProbe(test.runtime, test.version); got != test.want {
				t.Fatalf("runtime probe = %q, want %q", got, test.want)
			}
		})
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
	build := executionprofilemodel.Build{ID: "attempt", Runtime: "opencode", RuntimeVersion: "opencode v2.1.4", ResolvedBase: "example.com/base@" + hash, ResolvedToolchain: "example.com/tools@" + hash, Destination: "example.com/output:build-attempt"}
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
