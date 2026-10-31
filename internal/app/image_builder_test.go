package app

import (
	"testing"

	"github.com/josemiguelmelo/orkest/internal/executors/docker"
	"github.com/josemiguelmelo/orkest/internal/imagebuild/buildkit"
)

func TestDockerBuilderDoesNotRequireBuildctl(t *testing.T) {
	builder, err := newImageBuilder("docker", &docker.ImageStore{}, buildkit.Config{Binary: "/nonexistent/buildctl"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := builder.(*docker.ImageBuilder); !ok {
		t.Fatalf("wrong backend %T", builder)
	}
}

func TestImageBuilderSelectionRejectsUnknownAndKeepsBuildKitDefault(t *testing.T) {
	for _, kind := range []string{"", "buildkit"} {
		if _, err := newImageBuilder(kind, &docker.ImageStore{}, buildkit.Config{Binary: "/nonexistent/buildctl"}); err == nil {
			t.Fatal("BuildKit did not require its executable")
		}
	}
	if _, err := newImageBuilder("unsupported", &docker.ImageStore{}, buildkit.Config{}); err == nil {
		t.Fatal("unknown backend accepted")
	}
}
