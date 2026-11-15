// Package executors selects and constructs task-agent process executors.
package executors

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/checkout"
	dockerexecutor "github.com/josemiguelmelo/orkest/internal/adapters/executors/docker"
	kubernetesexecutor "github.com/josemiguelmelo/orkest/internal/adapters/executors/kubernetes"
	localexecutor "github.com/josemiguelmelo/orkest/internal/adapters/executors/local"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/server/coderepository"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
)

type Kind string

const (
	Docker     Kind = "docker"
	Local      Kind = "local"
	Kubernetes Kind = "kubernetes"
)

// Executor is the agent-control contract. Selection owns infrastructure cleanup.
type Executor interface {
	orchestration.AgentctlStarter
	orchestration.AgentProcessController
}

type EditorConfig struct{ Image, ReadOnlyImage string }

// Selection pairs the executor with its optional editor capabilities.
type Selection struct {
	Editor   taskeditor.Backend
	Kind     Kind
	Executor Executor
	close    func() error
}

// Close releases infrastructure resources after services have drained. It never
// terminates independent agent processes and is safe across copies of Selection.
func (s Selection) Close() error {
	if s.close == nil {
		return nil
	}
	return s.close()
}

func selection(kind Kind, executor Executor, editor taskeditor.Backend, cleanup func() error) Selection {
	return Selection{Kind: kind, Executor: executor, Editor: editor, close: sync.OnceValue(cleanup)}
}

// Config holds the shared settings needed to construct either executor.
type Config struct {
	Editor                   EditorConfig
	Kind                     string
	DataRoot                 string
	DockerNetwork            string
	DockerImages             map[coreruntime.Runtime]string
	CodexAuthFile            string
	AgentctlBinary           string
	ProviderEnv              []string
	Environment              map[string]string
	CodingProviders          []coderepository.CodingProvider
	CodingCredentialResolver checkout.CredentialResolver
	Kubernetes               kubernetesexecutor.Config
}

// New constructs the configured executor. An empty kind retains Docker as the default.
func New(ctx context.Context, config Config) (Selection, error) {
	kind := Kind(strings.ToLower(strings.TrimSpace(config.Kind)))
	if kind == "" {
		kind = Docker
	}
	switch kind {
	case Docker:
		engine, err := dockerexecutor.NewEngineClient()
		if err != nil {
			return Selection{}, fmt.Errorf("configure Docker Engine client: %w", err)
		}
		return newDockerSelection(ctx, config, engine)
	case Local:
		executor, err := localexecutor.New(localexecutor.Config{
			DataRoot: config.DataRoot, AgentctlBinary: config.AgentctlBinary,
			CodexAuthFile: config.CodexAuthFile, ProviderEnv: config.ProviderEnv,
			Environment:     config.Environment,
			CodingProviders: config.CodingProviders, CodingCredentialResolver: config.CodingCredentialResolver,
		})
		if err != nil {
			return Selection{}, err
		}
		return selection(kind, executor, nil, executor.Close), nil
	case Kubernetes:
		if strings.TrimSpace(config.CodexAuthFile) != "" {
			return Selection{}, fmt.Errorf("ORKEST_CODEX_AUTH_FILE is not supported with the Kubernetes executor; configure the Kubernetes Codex auth Secret and key")
		}
		config.Kubernetes.DataRoot = config.DataRoot
		config.Kubernetes.Images = config.DockerImages
		config.Kubernetes.ProviderEnv = config.ProviderEnv
		config.Kubernetes.Environment = config.Environment
		config.Kubernetes.CodingProviders = config.CodingProviders
		config.Kubernetes.CodingCredentialResolver = config.CodingCredentialResolver
		executor, err := kubernetesexecutor.New(ctx, config.Kubernetes)
		if err != nil {
			return Selection{}, err
		}
		return selection(kind, executor, nil, executor.Close), nil
	default:
		return Selection{}, fmt.Errorf("unsupported ORKEST_EXECUTOR %q (expected %q, %q, or %q)", config.Kind, Docker, Local, Kubernetes)
	}
}

// newDockerSelection takes ownership of engine, including on construction failure.
func newDockerSelection(ctx context.Context, config Config, engine dockerexecutor.Engine) (Selection, error) {
	executor, err := dockerexecutor.NewWithBorrowedClient(ctx, dockerexecutor.Config{
		DataRoot: config.DataRoot, NetworkName: config.DockerNetwork,
		CodexAuthFile: config.CodexAuthFile, Images: config.DockerImages,
		ProviderEnv: config.ProviderEnv, CodingProviders: config.CodingProviders,
		Environment:              config.Environment,
		CodingCredentialResolver: config.CodingCredentialResolver,
	}, engine)
	if err != nil {
		_ = engine.Close()
		return Selection{}, err
	}
	backend, err := dockerexecutor.NewEditorBackendWithClient(dockerexecutor.EditorConfig{
		DataRoot: config.DataRoot, NetworkName: config.DockerNetwork,
		Image: config.Editor.Image, ReadOnlyImage: config.Editor.ReadOnlyImage,
	}, engine)
	if err != nil {
		_ = engine.Close()
		return Selection{}, err
	}
	return selection(Docker, executor, backend, engine.Close), nil
}
