package main

import (
	"testing"

	"github.com/josemiguelmelo/orkest/internal/adapters/imagebuild/buildkit"
	dockerimage "github.com/josemiguelmelo/orkest/internal/adapters/imagebuild/docker"
)

func TestDockerBuilderDoesNotRequireBuildctl(t *testing.T) {
	builder, err := newImageBuilder("docker", &dockerimage.ImageStore{}, buildkit.Config{Binary: "/nonexistent/buildctl"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := builder.(*dockerimage.ImageBuilder); !ok {
		t.Fatalf("wrong backend %T", builder)
	}
}

func TestImageBuilderSelectionRejectsUnknownAndKeepsBuildKitDefault(t *testing.T) {
	for _, kind := range []string{"", "buildkit"} {
		if _, err := newImageBuilder(kind, &dockerimage.ImageStore{}, buildkit.Config{Binary: "/nonexistent/buildctl"}); err == nil {
			t.Fatal("BuildKit did not require its executable")
		}
	}
	if _, err := newImageBuilder("unsupported", &dockerimage.ImageStore{}, buildkit.Config{}); err == nil {
		t.Fatal("unknown backend accepted")
	}
}
