package docker

import (
	"context"
	"errors"

	"github.com/josemiguelmelo/orkest/internal/executionprofile"
	"github.com/josemiguelmelo/orkest/internal/imagebuild"
)

// ImageBuilder composes and publishes images through the deployment's Docker daemon.
type ImageBuilder struct{ store *ImageStore }

func NewImageBuilder(store *ImageStore) (*ImageBuilder, error) {
	if store == nil {
		return nil, errors.New("image store is required")
	}
	return &ImageBuilder{store: store}, nil
}

func (b *ImageBuilder) ResolveInputs(ctx context.Context, build executionprofile.Build) (executionprofile.Build, error) {
	return b.store.ResolveInputs(ctx, build)
}

func (b *ImageBuilder) BuildAndPush(ctx context.Context, build executionprofile.Build) (string, error) {
	prepared, err := imagebuild.Prepare(build)
	if err != nil {
		return "", executionprofile.Failure("prepare_context", "Could not prepare the image build context; verify the runtime and pinned image inputs.", err)
	}
	defer func() { _ = prepared.Close() }()
	if _, err = b.store.command(ctx, "build", "--tag", prepared.Destination, "--label", "dev.orkest.build="+build.ID, "--build-arg", "BASE="+build.ResolvedBase, "--build-arg", "TOOLCHAIN="+build.ResolvedToolchain, prepared.Directory); err != nil {
		return "", executionprofile.Failure("docker_build", commandReason(err, "Docker image build failed; check daemon access to source registries and package networks."), err)
	}
	// Validate before publishing when the artifact already lives on the execution daemon.
	if err = b.store.validate(ctx, prepared.Destination, build); err != nil {
		return "", executionprofile.Failure("validate_image", "Composed image failed runtime validation; check image compatibility and required executables.", err)
	}
	if _, err = b.store.command(ctx, "push", prepared.Destination); err != nil {
		return "", executionprofile.Failure("push_image", commandReason(err, "Docker could not push the composed image; check output registry reachability and credentials."), err)
	}
	// Verify the published digest as well, sharing reconciliation with BuildKit.
	return b.store.Verify(ctx, build, prepared.Destination)
}

func (b *ImageBuilder) Reconcile(ctx context.Context, build executionprofile.Build) (string, error) {
	return b.store.Verify(ctx, build, build.Destination)
}

var _ executionprofile.Builder = (*ImageBuilder)(nil)
