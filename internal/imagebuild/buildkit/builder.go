// Package buildkit composes and publishes execution images using BuildKit.
package buildkit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/distribution/reference"
	"github.com/josemiguelmelo/orkest/internal/executionprofile"
	"github.com/josemiguelmelo/orkest/internal/imagebuild"
	digest "github.com/opencontainers/go-digest"
)

// ImageStore resolves registry inputs and verifies/loads the published artifact for execution.
// The implementation is shared with Docker-based image composition.
type ImageStore interface {
	ResolveInputs(context.Context, executionprofile.Build) (executionprofile.Build, error)
	Verify(context.Context, executionprofile.Build, string) (string, error)
}

type Config struct {
	Binary     string
	Address    string
	Registry   imagebuild.RegistryMapping
	TLSCA      string
	TLSCert    string
	TLSKey     string
	Daemonless bool
}

type Builder struct {
	config Config
	store  ImageStore
}

func New(config Config, store ImageStore) (*Builder, error) {
	if store == nil {
		return nil, errors.New("image store is required")
	}
	if config.Binary == "" {
		config.Binary = "buildctl"
	}
	if config.Daemonless {
		config.Binary = "buildctl-daemonless.sh"
	} else if (config.TLSCert == "") != (config.TLSKey == "") {
		return nil, errors.New("buildkit client certificate and key must be configured together")
	}
	binary, err := exec.LookPath(config.Binary)
	if err != nil {
		return nil, errors.New("image composition requires buildctl")
	}
	config.Binary = binary
	return &Builder{config: config, store: store}, nil
}

func (b *Builder) ResolveInputs(ctx context.Context, build executionprofile.Build) (executionprofile.Build, error) {
	return b.store.ResolveInputs(ctx, build)
}

func (b *Builder) BuildAndPush(ctx context.Context, build executionprofile.Build) (string, error) {
	prepared, err := imagebuild.Prepare(build)
	if err != nil {
		return "", executionprofile.Failure("prepare_context", "Could not prepare the image build context; verify the runtime and pinned image inputs.", err)
	}
	defer func() { _ = prepared.Close() }()
	destination, err := reference.ParseNormalizedNamed(prepared.Destination)
	if err != nil {
		return "", executionprofile.Failure("prepare_destination", "The output image reference is invalid; verify the execution image repository setting.", err)
	}
	destinationName := b.config.Registry.ForBuildKit(destination.String())
	resolvedBase := b.config.Registry.ForBuildKit(build.ResolvedBase)
	resolvedToolchain := b.config.Registry.ForBuildKit(build.ResolvedToolchain)
	contextDir := prepared.Directory
	// Keep exporter metadata outside the sent build context.
	metadataPath := filepath.Join(prepared.Root, "metadata.json")
	var args []string
	if !b.config.Daemonless {
		args = b.connectionArgs()
	}
	args = append(args, "build", "--frontend", "dockerfile.v0", "--local", "context="+contextDir, "--local", "dockerfile="+contextDir,
		"--opt", "build-arg:BASE="+resolvedBase, "--opt", "build-arg:TOOLCHAIN="+resolvedToolchain,
		"--opt", "label:dev.orkest.build="+build.ID, "--output", "type=image,name="+destinationName+",push=true", "--metadata-file", metadataPath)
	cmd := exec.CommandContext(ctx, b.config.Binary, args...)
	cmd.WaitDelay = 5 * time.Second
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err = cmd.Run(); err != nil {
		return "", executionprofile.Failure("buildkit_build_push", "BuildKit build or push failed; check daemon connectivity, registry access, and package network access.", err)
	}
	data, err := os.Open(metadataPath)
	if err != nil {
		return "", executionprofile.Failure("read_export_metadata", "BuildKit completed without readable export metadata.", err)
	}
	defer func() { _ = data.Close() }()
	var metadata struct {
		Digest string `json:"containerimage.digest"`
	}
	if err = json.NewDecoder(io.LimitReader(data, 65536)).Decode(&metadata); err != nil {
		return "", executionprofile.Failure("read_export_metadata", "BuildKit returned invalid export metadata.", err)
	}
	hash, err := digest.Parse(metadata.Digest)
	if err != nil {
		return "", executionprofile.Failure("read_export_digest", "BuildKit did not return a valid image digest.", err)
	}
	canonical, err := reference.WithDigest(reference.TrimNamed(destination), hash)
	if err != nil {
		return "", executionprofile.Failure("prepare_export_reference", "BuildKit returned an invalid published image reference.", err)
	}
	verified, err := b.store.Verify(ctx, build, canonical.String())
	if err != nil {
		return "", err
	}
	actual, err := reference.ParseNormalizedNamed(verified)
	if err != nil || actual.String() != canonical.String() {
		return "", errors.New("validated image differs from buildkit export")
	}
	return canonical.String(), nil
}

func (b *Builder) connectionArgs() []string {
	var args []string
	for _, v := range []struct{ flag, value string }{{"--addr", b.config.Address}, {"--tlscacert", b.config.TLSCA}, {"--tlscert", b.config.TLSCert}, {"--tlskey", b.config.TLSKey}} {
		if v.value != "" {
			args = append(args, v.flag, v.value)
		}
	}
	return args
}

func (b *Builder) Reconcile(ctx context.Context, build executionprofile.Build) (string, error) {
	return b.store.Verify(ctx, build, build.Destination)
}

var _ executionprofile.Builder = (*Builder)(nil)
