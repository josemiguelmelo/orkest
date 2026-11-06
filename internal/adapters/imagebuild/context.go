// Package imagebuild provides the controlled build context shared by image builders.
package imagebuild

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/distribution/reference"
	dockerrecipes "github.com/josemiguelmelo/orkest/build/docker"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/server/executionprofile"
)

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$`)

//go:embed templates/execution-profile.Dockerfile.tmpl
var executionProfileDockerfile string

var executionProfileDockerfileTemplate = template.Must(template.New("execution-profile").Parse(executionProfileDockerfile))

type Context struct{ Root, Directory, Destination string }

func (c *Context) Close() error { return os.RemoveAll(c.Root) }

// Prepare validates the inputs before writing a minimal context without task data.
func Prepare(build executionprofile.Build) (*Context, error) {
	if _, ok := coreruntime.ProviderFor(build.Runtime); !ok {
		return nil, errors.New("unsupported runtime")
	}
	for _, input := range []string{build.ResolvedBase, build.ResolvedToolchain} {
		named, err := reference.ParseNormalizedNamed(input)
		if err != nil {
			return nil, errors.New("invalid resolved build input")
		}
		if _, ok := named.(reference.Digested); !ok {
			return nil, errors.New("image build inputs require registry digests")
		}
	}
	destination, err := reference.ParseNormalizedNamed(build.Destination)
	if err != nil {
		return nil, errors.New("invalid build destination")
	}
	version := strings.TrimPrefix(build.RuntimeVersion, "opencode v")
	if !versionPattern.MatchString(version) {
		return nil, errors.New("invalid runtime version")
	}
	root, err := os.MkdirTemp("", "orkest-image-")
	if err != nil {
		return nil, err
	}
	result := &Context{Root: root, Directory: filepath.Join(root, "context"), Destination: destination.String()}
	success := false
	defer func() {
		if !success {
			_ = result.Close()
		}
	}()
	if err = os.Mkdir(result.Directory, 0o700); err != nil {
		return nil, err
	}
	var dockerfile strings.Builder
	if err = executionProfileDockerfileTemplate.Execute(&dockerfile, struct {
		Runtime        coreruntime.Runtime
		RuntimeVersion string
	}{Runtime: build.Runtime, RuntimeVersion: version}); err != nil {
		return nil, fmt.Errorf("render execution profile Dockerfile: %w", err)
	}
	if err = os.WriteFile(filepath.Join(result.Directory, "Dockerfile"), []byte(dockerfile.String()), 0o600); err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(result.Directory, "install-runtime.sh"), dockerrecipes.InstallRuntime, 0o600); err != nil {
		return nil, err
	}
	success = true
	return result, nil
}
