package imagebuild

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/server/executionprofile"
)

func TestPrepareRendersEmbeddedDockerfileTemplate(t *testing.T) {
	build := executionprofile.Build{
		Runtime:           "opencode",
		RuntimeVersion:    "opencode v2.1.4",
		ResolvedBase:      "registry.example.com/base@sha256:" + strings.Repeat("a", 64),
		ResolvedToolchain: "registry.example.com/toolchain@sha256:" + strings.Repeat("b", 64),
		Destination:       "registry.example.com/output:build-1",
	}
	prepared, err := Prepare(build)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = prepared.Close() }()

	data, err := os.ReadFile(filepath.Join(prepared.Directory, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	dockerfile := string(data)
	for _, want := range []string{
		"FROM ${BASE} AS agentctl",
		"FROM ${TOOLCHAIN}",
		"COPY --from=agentctl /usr/local/bin/agentctl /usr/local/bin/agentctl",
		"RUN sh /tmp/orkest-install-runtime.sh opencode 2.1.4",
	} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("rendered Dockerfile does not contain %q", want)
		}
	}
}
