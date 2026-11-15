// Package executors selects and constructs task-agent process executors.
package executors

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	sharedagentctl "github.com/josemiguelmelo/orkest/internal/adapters/executors/agentctl"
	"github.com/josemiguelmelo/orkest/internal/adapters/executors/checkout"
	sharededitor "github.com/josemiguelmelo/orkest/internal/adapters/executors/editor"
	enginecontract "github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	dockerengine "github.com/josemiguelmelo/orkest/internal/adapters/executors/engine/docker"
	kubernetesengine "github.com/josemiguelmelo/orkest/internal/adapters/executors/engine/kubernetes"
	localengine "github.com/josemiguelmelo/orkest/internal/adapters/executors/engine/local"
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

// AgentCtlExecutor is the platform's shared worker-control contract.
// Selection separately owns the underlying infrastructure engine cleanup.
type AgentCtlExecutor interface {
	orchestration.AgentctlStarter
	orchestration.AgentProcessController
}

type EditorConfig struct{ Image, ReadOnlyImage string }

// Selection pairs the executor with its optional editor capabilities.
type Selection struct {
	EditorExecutor   taskeditor.EditorExecutor
	Kind             Kind
	AgentCtlExecutor AgentCtlExecutor
	close            func() error
}

// Close releases infrastructure resources after services have drained. It never
// terminates independent agent processes and is safe across copies of Selection.
func (s Selection) Close() error {
	if s.close == nil {
		return nil
	}
	return s.close()
}

func selection(kind Kind, executor AgentCtlExecutor, editor taskeditor.EditorExecutor, cleanup func() error) Selection {
	return Selection{Kind: kind, AgentCtlExecutor: executor, EditorExecutor: editor, close: sync.OnceValue(cleanup)}
}

// Config holds the shared settings needed to construct either executor.
type Config struct {
	Editor                   EditorConfig
	Kind                     string
	DataRoot                 string
	DockerNetwork            string
	RuntimeImages            map[coreruntime.Runtime]string
	CodexAuthFile            string
	AgentctlBinary           string
	ProviderEnv              []string
	Environment              map[string]string
	CodingProviders          []coderepository.CodingProvider
	CodingCredentialResolver checkout.CredentialResolver
	Kubernetes               kubernetesengine.EngineConfig
}

// New constructs the configured executor. An empty kind retains Docker as the default.
func New(ctx context.Context, config Config) (Selection, error) {
	kind := Kind(strings.ToLower(strings.TrimSpace(config.Kind)))
	if kind == "" {
		kind = Docker
	}
	switch kind {
	case Docker:
		engine, err := dockerengine.NewClient()
		if err != nil {
			return Selection{}, fmt.Errorf("configure Docker Engine client: %w", err)
		}
		return newDockerSelection(ctx, config, engine)
	case Local:
		localEngine, err := localengine.New(localengine.Config{DataRoot: config.DataRoot})
		if err != nil {
			return Selection{}, err
		}
		providers, providerErr := repositoryProviders(config.CodingProviders)
		if providerErr != nil {
			_ = localEngine.Close()
			return Selection{}, providerErr
		}
		executor, err := sharedagentctl.NewSharedAgentCtlExecutor(sharedagentctl.ExecutorConfig{
			Engine: localEngine, Mode: enginecontract.ExecutionModeNative,
			AgentctlBinary: config.AgentctlBinary, CodexAuthFile: config.CodexAuthFile,
			ProviderEnv: config.ProviderEnv, Environment: config.Environment,
			Repositories: sharedagentctl.WorkspaceConfig{Repositories: checkout.Preparer{
				Providers: providers, Credentials: config.CodingCredentialResolver,
			}},
		})
		if err != nil {
			_ = localEngine.Close()
			return Selection{}, err
		}
		editorExecutor, err := newSharedEditorExecutor(localEngine, config.Editor)
		if err != nil {
			_ = localEngine.Close()
			return Selection{}, err
		}
		return selection(kind, executor, editorExecutor, localEngine.Close), nil
	case Kubernetes:
		if strings.TrimSpace(config.CodexAuthFile) != "" {
			return Selection{}, fmt.Errorf("ORKEST_CODEX_AUTH_FILE is not supported with the Kubernetes executor; configure the Kubernetes Codex auth Secret and key")
		}
		return newKubernetesSelection(ctx, config)
	default:
		return Selection{}, fmt.Errorf("unsupported ORKEST_EXECUTOR %q (expected %q, %q, or %q)", config.Kind, Docker, Local, Kubernetes)
	}
}

func newKubernetesSelection(ctx context.Context, config Config) (Selection, error) {
	if ctx == nil {
		return Selection{}, errors.New("executor construction context is required")
	}
	if err := ctx.Err(); err != nil {
		return Selection{}, err
	}
	kubernetesConfig := config.Kubernetes
	kubernetesConfig.DataRoot = config.DataRoot
	api := kubernetesConfig.Client
	if api == nil {
		var err error
		api, err = kubernetesengine.NewInClusterAPI()
		if err != nil {
			return Selection{}, err
		}
	}
	uid, gid := kubernetesConfig.UID, kubernetesConfig.GID
	if uid == 0 {
		uid = int64(os.Getuid())
	}
	if gid == 0 {
		gid = int64(os.Getgid())
	}
	infrastructure, err := kubernetesengine.New(kubernetesengine.EngineConfig{
		DataRoot: kubernetesConfig.DataRoot, DataSubpath: kubernetesConfig.DataSubpath,
		Namespace: kubernetesConfig.Namespace, InstallationID: kubernetesConfig.InstallationID,
		DataPVC: kubernetesConfig.DataPVC, ServiceAccount: kubernetesConfig.ServiceAccount,
		CodexAuthSecret: kubernetesConfig.CodexAuthSecret, CodexAuthSecretKey: kubernetesConfig.CodexAuthSecretKey,
		ImagePullSecrets: kubernetesConfig.ImagePullSecrets,
		CPURequest:       kubernetesConfig.CPURequest, MemoryRequest: kubernetesConfig.MemoryRequest,
		CPULimit: kubernetesConfig.CPULimit, MemoryLimit: kubernetesConfig.MemoryLimit,
		UID: uid, GID: gid, TerminationGraceSeconds: kubernetesConfig.TerminationGraceSeconds,
		StartupTimeout: kubernetesConfig.StartupTimeout, Client: api,
	})
	if err != nil {
		return Selection{}, err
	}
	providers, providerErr := repositoryProviders(config.CodingProviders)
	if providerErr != nil {
		_ = infrastructure.Close()
		return Selection{}, providerErr
	}
	executor, err := sharedagentctl.NewSharedAgentCtlExecutor(sharedagentctl.ExecutorConfig{
		Engine: infrastructure, Mode: enginecontract.ExecutionModeContainer,
		Images: config.RuntimeImages, ProviderEnv: config.ProviderEnv, Environment: config.Environment,
		CodexAuthBinding: kubernetesConfig.CodexAuthSecret != "",
		Repositories:     sharedagentctl.WorkspaceConfig{Repositories: checkout.Preparer{Providers: providers, Credentials: config.CodingCredentialResolver}},
	})
	if err != nil {
		_ = infrastructure.Close()
		return Selection{}, err
	}
	editorExecutor, err := newSharedEditorExecutor(infrastructure, config.Editor)
	if err != nil {
		_ = infrastructure.Close()
		return Selection{}, err
	}
	return selection(Kubernetes, executor, editorExecutor, infrastructure.Close), nil
}

func newSharedEditorExecutor(infrastructure enginecontract.ExecutorEngine, config EditorConfig) (*sharededitor.EditorExecutor, error) {
	uid, gid := os.Getuid(), os.Getgid()
	if uid == 0 || gid == 0 {
		uid, gid = 1000, 1000
	}
	return sharededitor.New(infrastructure, sharededitor.Config{Image: config.Image, ReadOnlyImage: config.ReadOnlyImage, RunAsUser: uint32(uid), RunAsGroup: uint32(gid)})
}

// newDockerSelection takes ownership of engine, including on construction failure.
func newDockerSelection(ctx context.Context, config Config, engine dockerengine.Client) (Selection, error) {
	infrastructure, err := dockerengine.New(dockerengine.Config{
		DataRoot: config.DataRoot, NetworkName: config.DockerNetwork,
		CodexAuthFile: config.CodexAuthFile, Client: engine,
	})
	if err != nil {
		_ = engine.Close()
		return Selection{}, err
	}
	providers, providerErr := repositoryProviders(config.CodingProviders)
	if providerErr != nil {
		_ = infrastructure.Close()
		return Selection{}, providerErr
	}
	executor, err := sharedagentctl.NewSharedAgentCtlExecutor(sharedagentctl.ExecutorConfig{
		Engine: infrastructure, Mode: enginecontract.ExecutionModeContainer,
		Images: config.RuntimeImages, CodexAuthFile: config.CodexAuthFile,
		ProviderEnv: config.ProviderEnv, Environment: config.Environment,
		Repositories: sharedagentctl.WorkspaceConfig{Repositories: checkout.Preparer{
			Providers: providers, Credentials: config.CodingCredentialResolver,
		}},
	})
	if err != nil {
		_ = infrastructure.Close()
		return Selection{}, err
	}
	editorExecutor, err := newSharedEditorExecutor(infrastructure, config.Editor)
	if err != nil {
		_ = infrastructure.Close()
		return Selection{}, err
	}
	return selection(Docker, executor, editorExecutor, infrastructure.Close), nil
}

func repositoryProviders(configured []coderepository.CodingProvider) (map[string]checkout.Provider, error) {
	providers := make(map[string]checkout.Provider, len(configured))
	for _, provider := range configured {
		if provider == nil || strings.TrimSpace(provider.ProviderName()) == "" {
			return nil, errors.New("code repository provider name is required")
		}
		providers[provider.ProviderName()] = provider
	}
	return providers, nil
}
