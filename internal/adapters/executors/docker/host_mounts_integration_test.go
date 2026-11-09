//go:build docker

package docker

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	"github.com/josemiguelmelo/orkest/internal/adapters/runnerclient/agentctlhttp"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/platform/sessionpath"
	mountmodel "github.com/josemiguelmelo/orkest/internal/server/workspacemount/model"
	mobyclient "github.com/moby/moby/client"
)

func TestDockerHostMountsReadWriteAndReplacement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	root, err := filepath.Abs("../../../../tmp")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	dataRoot, err := os.MkdirTemp(root, "docker-host-mounts-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dataRoot); err != nil {
			t.Error(err)
		}
	})
	input, output := filepath.Join(dataRoot, "input"), filepath.Join(dataRoot, "output")
	for _, directory := range []string{input, output} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(input, "data.txt")
	if err := os.WriteFile(file, []byte("host-input"), 0o600); err != nil {
		t.Fatal(err)
	}
	properties := filepath.Join(input, "gradle.properties")
	if err := os.WriteFile(properties, []byte("fixtureCredential=host-input\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(input, "config"), []byte("[profile fixture]\nregion = us-east-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := runConfig()
	config.TaskSessionID = "host-mount-session"
	config.HostMounts = []mountmodel.Mount{
		{Source: input, Target: "/mnt/host/input", ReadOnly: true},
		{Source: output, Target: "/mnt/host/output"},
		{Source: file, Target: "/mnt/host/file", ReadOnly: true},
		{Source: input, Target: "~/.aws", ReadOnly: true},
		{Source: file, Target: "~/.m2/settings.xml", ReadOnly: true},
		{Source: properties, Target: "~/.gradle/gradle.properties", ReadOnly: true},
	}
	image := os.Getenv("AGENTCTL_OPENCODE_IMAGE")
	toolchain := os.Getenv("ORKEST_DOCKER_TOOLCHAIN_TEST_IMAGE")
	if toolchain != "" {
		image = toolchain
	}
	executor, err := New(ctx, Config{
		DataRoot: dataRoot,
		Images:   map[coreruntime.Runtime]string{coreruntime.OpenCode: image},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = executor.Close() }()
	for attempt := range 2 {
		process, err := executor.StartAgentctl(ctx, config)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
			defer stop()
			if _, err := executor.client.ContainerRemove(cleanupCtx, process.ID, mobyclient.ContainerRemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
				t.Error(err)
			}
		})
		if err := agentctlhttp.NewHTTPAgentctlClient(&http.Client{Timeout: 2 * time.Second}).WaitHealthy(ctx, process.Endpoint); err != nil {
			t.Fatal(err)
		}
		script := `set -eu
test "$(cat /mnt/host/input/data.txt)" = host-input
test "$(cat /mnt/host/file)" = host-input
test "$(cat "$HOME/.aws/data.txt")" = host-input
test "$(cat "$HOME/.m2/settings.xml")" = host-input
test "$(cat "$HOME/.gradle/gradle.properties")" = fixtureCredential=host-input
if echo changed > "$HOME/.m2/settings.xml"; then exit 1; fi
if echo changed > "$HOME/.gradle/gradle.properties"; then exit 1; fi
echo writable-cache > "$HOME/.gradle/cache-test"
if echo changed > /mnt/host/input/data.txt; then exit 1; fi
if echo changed > /mnt/host/file; then exit 1; fi
echo host-output > /mnt/host/output/result.txt`
		home := sessionpath.Home(stateMountPath, config.OrganizationID, config.WorkspaceID, config.TaskID, config.TaskSessionID)
		if result, err := exec.CommandContext(ctx, "docker", "exec", "-e", "HOME="+home, process.ID, "/bin/sh", "-c", script).CombinedOutput(); err != nil {
			t.Fatalf("check bind mount access: %v: %s", err, result)
		}
		t.Run("tool_discovery", func(t *testing.T) {
			if toolchain == "" {
				t.Skip("set ORKEST_DOCKER_TOOLCHAIN_TEST_IMAGE to a JVM/infrastructure image with agentctl, AWS CLI and Gradle")
			}
			workDir, _, err := executor.taskDirectories(config)
			if err != nil {
				t.Fatal(err)
			}
			build := `tasks.register('checkCredentialMount') {
    doLast {
        assert providers.gradleProperty('fixtureCredential').get() == 'host-input'
        println 'credential-file-discovered'
    }
}
`
			if err := os.WriteFile(filepath.Join(workDir, "build.gradle"), []byte(build), 0o600); err != nil {
				t.Fatal(err)
			}
			script := `set -eu
aws configure list-profiles | grep -Fx fixture
gradle --offline --no-daemon --gradle-user-home "$HOME/.gradle" -p /workspace checkCredentialMount`
			if result, err := exec.CommandContext(ctx, "docker", "exec", "-e", "HOME="+home, process.ID, "/bin/sh", "-c", script).CombinedOutput(); err != nil {
				t.Fatalf("tools did not discover mounted configuration: %v: %s", err, result)
			}
		})
		if content, err := os.ReadFile(filepath.Join(output, "result.txt")); err != nil || string(content) != "host-output\n" {
			t.Fatal("container write did not reach host", err)
		}
		if content, err := os.ReadFile(file); err != nil || string(content) != "host-input" {
			t.Fatal("read-only host input changed", err)
		}
		if _, err := executor.client.ContainerRemove(ctx, process.ID, mobyclient.ContainerRemoveOptions{Force: true}); err != nil {
			t.Fatal(err)
		}
		if attempt == 0 {
			config.RunID = "host-mount-replacement"
			config.RestoreSession = true
		}
	}
	missing := filepath.Join(dataRoot, "missing")
	config.RunID = "host-mount-missing"
	config.HostMounts = []mountmodel.Mount{{Source: missing, Target: "/mnt/host/missing", ReadOnly: true}}
	process, startErr := executor.StartAgentctl(ctx, config)
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if process.ID != "" {
			if _, err := executor.client.ContainerRemove(cleanupCtx, process.ID, mobyclient.ContainerRemoveOptions{Force: true}); err != nil && !errdefs.IsNotFound(err) {
				t.Error(err)
			}
		}
	})
	if startErr == nil || process.Endpoint != "" {
		t.Fatal("missing host source did not fail before container start", startErr)
	}
	if _, err := executor.client.ContainerInspect(ctx, process.ID, mobyclient.ContainerInspectOptions{}); !errdefs.IsNotFound(err) {
		t.Fatal("missing host source unexpectedly created a container", err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("missing host source was automatically created", err)
	}
}
