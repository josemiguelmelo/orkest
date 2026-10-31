package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/distribution/reference"
	coreruntime "github.com/josemiguelmelo/orkest/internal/core/runtime"
	"github.com/josemiguelmelo/orkest/internal/executionprofile"
	"github.com/josemiguelmelo/orkest/internal/imagebuild"
)

func currentContainerUser() (string, error) {
	current, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("resolve current user for image validation: %w", err)
	}
	if _, err = strconv.Atoi(current.Uid); err != nil {
		return "", errors.New("image validation requires a numeric local user ID")
	}
	if _, err = strconv.Atoi(current.Gid); err != nil {
		return "", errors.New("image validation requires a numeric local group ID")
	}
	return current.Uid + ":" + current.Gid, nil
}

// ImageStore resolves registry inputs and validates published images using Docker.
// Commands receive no workspace secrets and their raw output is never published.
type ImageStore struct {
	binary   string
	user     string
	registry imagebuild.RegistryMapping
}

func NewImageStore(registry imagebuild.RegistryMapping) (*ImageStore, error) {
	user, err := currentContainerUser()
	if err != nil {
		return nil, err
	}
	binary, err := exec.LookPath("docker")
	if err != nil {
		return nil, errors.New("image validation requires Docker CLI")
	}
	return &ImageStore{binary: binary, user: user, registry: registry}, nil
}

func (b *ImageStore) command(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, b.binary, args...)
	cmd.WaitDelay = 5 * time.Second
	var out limitedOutput
	var stderr limitedOutput
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, &commandFailure{reason: classifyDockerFailure(stderr.String() + "\n" + out.String())}
	}
	return out.Bytes(), nil
}

type commandFailure struct {
	reason string
}

func (e *commandFailure) Error() string { return e.reason }

func classifyDockerFailure(stderr string) string {
	message := strings.ToLower(stderr)
	switch {
	case strings.Contains(message, "unauthorized"), strings.Contains(message, "authentication required"), strings.Contains(message, "denied: requested access"):
		return "Docker was denied registry access; verify credentials available to the Docker daemon."
	case strings.Contains(message, "connection refused"):
		return "A registry connection was refused; verify registry address, port, and daemon network access."
	case strings.Contains(message, "no such host"), strings.Contains(message, "name or service not known"), strings.Contains(message, "temporary failure in name resolution"):
		return "Docker could not resolve a registry hostname; verify DNS from the Docker daemon."
	case strings.Contains(message, "manifest unknown"), strings.Contains(message, "manifest for "), strings.Contains(message, "not found"):
		return "A required image or manifest was not found; verify source image references and tags."
	case strings.Contains(message, "failed to load metadata for"), strings.Contains(message, "pull access denied"), strings.Contains(message, "failed to authorize"):
		return "Docker could not load a source image; verify source references, registry reachability, and daemon credentials."
	case strings.Contains(message, "no match for platform"), strings.Contains(message, "platform does not match"):
		return "A source image does not support the Docker daemon platform; use compatible image architectures."
	case strings.Contains(message, "failed to compute cache key"), strings.Contains(message, "failed to calculate checksum"):
		return "Docker could not prepare files or source layers for the build; verify the source image contents and build context."
	case strings.Contains(message, "failed to solve: process"), strings.Contains(message, "returned a non-zero code"), strings.Contains(message, "exit code:"):
		return "A Dockerfile build step failed; check runtime download/package compatibility and network access."
	case strings.Contains(message, "cannot connect to the docker daemon"), strings.Contains(message, "error during connect"):
		return "Docker CLI could not communicate with the Docker daemon; verify the daemon socket and permissions."
	case strings.Contains(message, "x509"), strings.Contains(message, "certificate"):
		return "Docker rejected a registry certificate; verify registry TLS trust and certificate configuration."
	case strings.Contains(message, "no space left"):
		return "The Docker daemon ran out of storage while composing the image."
	case strings.Contains(message, "i/o timeout"), strings.Contains(message, "deadline exceeded"), strings.Contains(message, "tls handshake timeout"), strings.Contains(message, "network is unreachable"):
		return "Docker could not complete a network operation; verify registry and package network access."
	default:
		return "Docker image command failed; inspect Docker daemon diagnostics for the underlying error."
	}
}

func commandReason(err error, fallback string) string {
	var commandErr *commandFailure
	if errors.As(err, &commandErr) {
		return commandErr.reason
	}
	return fallback
}

type limitedOutput struct{ bytes.Buffer }

func (w *limitedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 65536 - w.Len()
	if remaining > 0 {
		_, _ = w.Buffer.Write(p[:min(n, remaining)])
	}
	return n, nil
}

type inspectedImage struct {
	ID          string
	RepoDigests []string
	Config      struct{ Labels map[string]string }
}

func (b *ImageStore) inspect(ctx context.Context, image string) (inspectedImage, error) {
	out, err := b.command(ctx, "image", "inspect", b.registry.ForDocker(image))
	if err != nil {
		return inspectedImage{}, err
	}
	var rows []inspectedImage
	if err = json.Unmarshal(out, &rows); err != nil || len(rows) != 1 {
		return inspectedImage{}, errors.New("invalid image inspection")
	}
	return rows[0], nil
}

func (b *ImageStore) artifact(ctx context.Context, build executionprofile.Build, ref string) (string, error) {
	image, err := b.inspect(ctx, ref)
	if err != nil {
		return "", err
	}
	if image.Config.Labels["dev.orkest.build"] != build.ID {
		return "", errors.New("build identity mismatch")
	}
	repository, _, _ := strings.Cut(build.Destination, ":build-")
	expected, err := reference.ParseNormalizedNamed(repository)
	if err != nil {
		return "", err
	}
	requested, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return "", err
	}
	requestedDigest, _ := requested.(reference.Digested)
	for _, digest := range image.RepoDigests {
		actual, parseErr := reference.ParseNormalizedNamed(digest)
		if parseErr == nil && reference.TrimNamed(actual).String() == expected.String() {
			if actualDigest, ok := actual.(reference.Digested); ok {
				if requestedDigest == nil || requestedDigest.Digest() == actualDigest.Digest() {
					return digest, nil
				}
			}
		}
	}
	return "", errors.New("pushed image digest unavailable")
}

func (b *ImageStore) validate(ctx context.Context, image string, build executionprofile.Build) error {
	name := "orkest-validate-" + build.ID
	args := []string{"run", "--name", name, "--rm", "--network", "none", "--memory", "512m", "--pids-limit", "128", "--cpus", "1", "--user", b.user, "--env", "HOME=/agentctl-state", "--env", "XDG_DATA_HOME=/agentctl-state/data", "--env", "XDG_CACHE_HOME=/agentctl-state/cache", "--tmpfs", "/workspace:mode=1777", "--tmpfs", "/agentctl-state:mode=1777", "--entrypoint", "/bin/sh", image, "-c", "test -x /usr/local/bin/agentctl && test -w /workspace && test -w /agentctl-state && " + runtimeProbe(build.Runtime, build.RuntimeVersion)}
	_, err := b.command(ctx, args...)
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, _ = b.command(cleanupCtx, "rm", "--force", name)
	return err
}

func runtimeProbe(runtime coreruntime.Runtime, version string) string {
	if runtime == coreruntime.OpenCode {
		return "test \"$(opencode --version)\" = '" + version + "'"
	}
	return "test \"$(claude-agent-acp --version)\" = '" + version + "'"
}

func (b *ImageStore) Verify(ctx context.Context, build executionprofile.Build, ref string) (string, error) {
	if _, err := b.command(ctx, "pull", b.registry.ForDocker(ref)); err != nil {
		return "", executionprofile.Failure("verify_pull_image", commandReason(err, "Docker could not pull the image for verification; check registry reachability and credentials."), err)
	}
	image, err := b.artifact(ctx, build, ref)
	if err != nil {
		return "", executionprofile.Failure("verify_image_identity", commandReason(err, "The published image is missing the expected build identity or registry digest."), err)
	}
	if err = b.validate(ctx, image, build); err != nil {
		return "", executionprofile.Failure("validate_published_image", commandReason(err, "The published image failed runtime validation; check image compatibility."), err)
	}
	return image, nil
}

func (b *ImageStore) ResolveInputs(ctx context.Context, build executionprofile.Build) (executionprofile.Build, error) {
	resolve := func(ref string) (string, error) {
		dockerRef := b.registry.ForDocker(ref)
		_, pullErr := b.command(ctx, "pull", dockerRef)
		image, err := b.inspect(ctx, dockerRef)
		if err != nil {
			return "", err
		}
		if pullErr != nil {
			return "", pullErr
		}
		requested, err := reference.ParseNormalizedNamed(dockerRef)
		if err != nil {
			return "", errors.New("invalid source image reference")
		}
		for _, repoDigest := range image.RepoDigests {
			actual, parseErr := reference.ParseNormalizedNamed(repoDigest)
			if parseErr != nil || reference.TrimNamed(actual).String() != reference.TrimNamed(requested).String() {
				continue
			}
			if _, ok := actual.(reference.Digested); ok {
				return repoDigest, nil
			}
		}
		if len(image.RepoDigests) == 0 {
			return "", errors.New("image build inputs must be published to a registry")
		}
		return "", errors.New("source image has no digest for the requested registry repository")
	}
	var err error
	build.ResolvedToolchain, err = resolve(build.ToolchainImage)
	if err != nil {
		return build, executionprofile.Failure("resolve_toolchain_image", commandReason(err, "Could not pull or resolve the toolchain image; check its registry reference and Docker registry access."), err)
	}
	build.ResolvedBase, err = resolve(build.BaseImage)
	if err != nil {
		return build, executionprofile.Failure("resolve_base_image", commandReason(err, "Could not pull or resolve the runtime base image; check its registry reference and Docker registry access."), err)
	}
	return build, err
}
