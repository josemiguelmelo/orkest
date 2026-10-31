// Package imagebuild provides the controlled build context shared by image builders.
package imagebuild

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/distribution/reference"
	dockerrecipes "github.com/josemiguelmelo/orkest/build/docker"
	coreruntime "github.com/josemiguelmelo/orkest/internal/core/runtime"
	"github.com/josemiguelmelo/orkest/internal/executionprofile"
)

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$`)

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
	dockerfile := fmt.Sprintf("ARG BASE\nARG TOOLCHAIN\nFROM ${BASE} AS agentctl\nFROM ${TOOLCHAIN}\nUSER root\nCOPY --from=agentctl /usr/local/bin/agentctl /usr/local/bin/agentctl\nCOPY install-runtime.sh /tmp/orkest-install-runtime.sh\nRUN sh /tmp/orkest-install-runtime.sh %s %s && rm /tmp/orkest-install-runtime.sh\nWORKDIR /workspace\nENTRYPOINT [\"/usr/local/bin/agentctl\"]\nCMD []\n", build.Runtime, version)
	if err = os.WriteFile(filepath.Join(result.Directory, "Dockerfile"), []byte(dockerfile), 0o600); err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(result.Directory, "install-runtime.sh"), dockerrecipes.InstallRuntime, 0o600); err != nil {
		return nil, err
	}
	success = true
	return result, nil
}
