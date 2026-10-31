//go:build docker

package docker

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/core/ids"
	"github.com/josemiguelmelo/orkest/internal/executionprofile"
	"github.com/josemiguelmelo/orkest/internal/imagebuild/buildkit"
)

// TestDockerImageComposition requires buildctl, registry:3, moby/buildkit:v0.34.1, and the built base image.
func TestDockerImageComposition(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	builder, err := NewImageStore()
	if err != nil {
		t.Fatal(err)
	}
	id, err := ids.NewUUID()
	if err != nil {
		t.Fatal(err)
	}
	name := "orkest-registry-test-" + id
	if _, err = builder.command(ctx, "run", "--detach", "--name", name, "--publish", "127.0.0.1::5000", "registry:3"); err != nil {
		t.Fatal("start test registry", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		_, _ = builder.command(cleanup, "rm", "--force", name)
	})
	portOutput, err := builder.command(ctx, "port", name, "5000/tcp")
	if err != nil {
		t.Fatal(err)
	}
	_, port, err := net.SplitHostPort(strings.TrimSpace(string(portOutput)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = strconv.Atoi(port); err != nil {
		t.Fatal(err)
	}
	registry := "127.0.0.1:" + port
	base := os.Getenv("AGENTCTL_BASE_IMAGE")
	if base == "" {
		base = "orkest/agentctl-base:dev"
	}
	source := registry + "/toolchain:base"
	toolDir := t.TempDir()
	if err = os.WriteFile(filepath.Join(toolDir, "Dockerfile"), []byte("FROM "+base+"\nRUN touch /opt/orkest-toolchain-marker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = builder.command(ctx, "build", "--tag", source, toolDir); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 20; attempt++ {
		if _, err = builder.command(ctx, "push", source); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
	if err != nil {
		t.Fatal("push source", err)
	}

	t.Run("docker", func(t *testing.T) {
		composer, err := NewImageBuilder(builder)
		if err != nil {
			t.Fatal(err)
		}
		testPublishedComposition(t, ctx, builder, composer, registry, source, id+"-docker")
	})
	t.Run("buildkit", func(t *testing.T) {
		// A separate daemon proves builds do not use the Docker image store/build API.
		buildkitName := "orkest-buildkit-test-" + id
		configDir := t.TempDir()
		if err = os.WriteFile(filepath.Join(configDir, "buildkitd.toml"), []byte("[registry.\""+registry+"\"]\n  http = true\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err = builder.command(ctx, "create", "--privileged", "--network", "host", "--name", buildkitName, "moby/buildkit:v0.34.1", "--config", "/buildkitd.toml"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
			defer stop()
			_, _ = builder.command(cleanup, "rm", "--force", buildkitName)
		})
		if _, err = builder.command(ctx, "cp", filepath.Join(configDir, "buildkitd.toml"), buildkitName+":/buildkitd.toml"); err != nil {
			t.Fatal(err)
		}
		if _, err = builder.command(ctx, "start", buildkitName); err != nil {
			t.Fatal(err)
		}
		for attempt := 0; attempt < 40; attempt++ {
			if _, err = builder.command(ctx, "exec", buildkitName, "buildctl", "debug", "workers"); err == nil {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(250 * time.Millisecond):
			}
		}
		if err != nil {
			t.Fatal("buildkit readiness", err)
		}
		composer, err := buildkit.New(buildkit.Config{Binary: os.Getenv("ORKEST_BUILDCTL_BINARY"), Address: "docker-container://" + buildkitName}, builder)
		if err != nil {
			t.Fatal(err)
		}
		testPublishedComposition(t, ctx, builder, composer, registry, source, id+"-buildkit")
	})
}

func testPublishedComposition(t *testing.T, ctx context.Context, builder *ImageStore, composer executionprofile.Builder, registry, source, id string) {
	t.Helper()
	build := executionprofile.Build{ID: id, Runtime: "opencode", RuntimeVersion: "opencode v2.1.4", ToolchainImage: source, BaseImage: source, Destination: registry + "/execution:build-" + id}
	build, err := composer.ResolveInputs(ctx, build)
	if err != nil {
		t.Fatal("resolve", err)
	}
	artifact, err := composer.BuildAndPush(ctx, build)
	if err != nil {
		t.Fatal("compose", err)
	}
	if !strings.Contains(artifact, "@sha256:") {
		t.Fatal("missing immutable output", artifact)
	}
	recovered, err := composer.Reconcile(ctx, build)
	if err != nil || recovered != artifact {
		t.Fatal("reconcile", recovered, err)
	}
	// Start a separate replacement from the exact artifact to exercise stored identity.
	if _, err = builder.command(ctx, "run", "--rm", "--network", "none", "--env", "HOME=/tmp", "--entrypoint", "/bin/sh", artifact, "-c", "test -f /opt/orkest-toolchain-marker && opencode --version"); err != nil {
		t.Fatal(fmt.Errorf("replacement runtime probe: %w", err))
	}
}
