// Package executors selects and constructs task-agent process executors.
package executors

import (
	"context"
	"fmt"
	"strings"

	dockerexecutor "github.com/josemiguelmelo/orkest/internal/adapters/executors/docker"
	localexecutor "github.com/josemiguelmelo/orkest/internal/adapters/executors/local"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/server/coderepository"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
)

type Kind string

const (
	Docker Kind = "docker"
	Local  Kind = "local"
)

// Executor is the application-facing lifecycle contract for an executor.
type Executor interface {
	orchestration.AgentctlStarter
	orchestration.AgentProcessController
	Close() error
}

// Selection pairs the constructed executor with its selected execution mode.
type Selection struct {
	Kind     Kind
	Executor Executor
}

// Config holds the shared settings needed to construct either executor.
type Config struct {
	Kind                     string
	DataRoot                 string
	DockerNetwork            string
	DockerImages             map[coreruntime.Runtime]string
	CodexAuthFile            string
	AgentctlBinary           string
	ProviderEnv              []string
	CodingProviders          []coderepository.CodingProvider
	CodingCredentialResolver interface {
		CodingCredential(context.Context, string, string) (coderepository.Credentials, error)
	}
}

// New constructs the configured executor. An empty kind retains Docker as the default.
func New(ctx context.Context, config Config) (Selection, error) {
	kind := Kind(strings.ToLower(strings.TrimSpace(config.Kind)))
	if kind == "" {
		kind = Docker
	}
	switch kind {
	case Docker:
		executor, err := dockerexecutor.New(ctx, dockerexecutor.Config{
			DataRoot: config.DataRoot, NetworkName: config.DockerNetwork,
			CodexAuthFile: config.CodexAuthFile, Images: config.DockerImages,
			ProviderEnv: config.ProviderEnv, CodingProviders: config.CodingProviders,
			CodingCredentialResolver: config.CodingCredentialResolver,
		})
		if err != nil {
			return Selection{}, err
		}
		return Selection{Kind: kind, Executor: executor}, nil
	case Local:
		executor, err := localexecutor.New(localexecutor.Config{
			DataRoot: config.DataRoot, AgentctlBinary: config.AgentctlBinary,
			CodexAuthFile: config.CodexAuthFile, ProviderEnv: config.ProviderEnv,
			CodingProviders: config.CodingProviders, CodingCredentialResolver: config.CodingCredentialResolver,
		})
		if err != nil {
			return Selection{}, err
		}
		return Selection{Kind: kind, Executor: executor}, nil
	default:
		return Selection{}, fmt.Errorf("unsupported ORKEST_EXECUTOR %q (expected %q or %q)", config.Kind, Docker, Local)
	}
}
