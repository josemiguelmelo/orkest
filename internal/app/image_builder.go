package app

import (
	"fmt"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/executionprofile"
	"github.com/josemiguelmelo/orkest/internal/executors/docker"
	"github.com/josemiguelmelo/orkest/internal/imagebuild/buildkit"
)

// Select only at the application boundary; workers depend on executionprofile.Builder.
func newImageBuilder(kind string, store *docker.ImageStore, config buildkit.Config) (executionprofile.Builder, error) {
	switch strings.TrimSpace(kind) {
	case "", "buildkit":
		return buildkit.New(config, store)
	case "docker":
		return docker.NewImageBuilder(store)
	default:
		return nil, fmt.Errorf("unsupported ORKEST_IMAGE_BUILDER %q: use buildkit or docker", kind)
	}
}
