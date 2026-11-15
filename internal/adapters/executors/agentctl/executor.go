package agentctl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/checkout"
	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	runtimecatalog "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/platform/sessionpath"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	mountmodel "github.com/josemiguelmelo/orkest/internal/server/workspacemount/model"
)

const (
	codexAuthEnvironment   = "ORKEST_CODEX_AUTH_FILE"
	codexContainerAuthPath = "/agentctl-state/codex-auth/auth.json"
)

type ExecutorConfig struct {
	Engine           engine.ExecutorEngine
	Mode             engine.ExecutionMode
	AgentctlBinary   string
	Images           map[runtimecatalog.Runtime]string
	CodexAuthFile    string
	CodexAuthBinding bool
	ProviderEnv      []string
	Environment      map[string]string
	Repositories     WorkspaceConfig
}

// AgentCtlExecutor owns shared agent launch planning and process control.
// Concrete engines realize the generated launch requests and own infrastructure.
type AgentCtlExecutor struct {
	workspace WorkspaceConfig
	engine    engine.ExecutorEngine
	config    ExecutorConfig
}

// NewSharedAgentCtlExecutor constructs the shared run planner and controller.
// The selected engine is borrowed; its composition owner closes it.
func NewSharedAgentCtlExecutor(config ExecutorConfig) (*AgentCtlExecutor, error) {
	if config.Engine == nil {
		return nil, errors.New("agentctl executor engine is required")
	}
	if config.Mode != engine.ExecutionModeContainer && config.Mode != engine.ExecutionModeNative {
		return nil, errors.New("agentctl execution mode is required")
	}
	if !hasMode(config.Engine.Capabilities(), config.Mode) {
		return nil, engine.Unsupported("configured agent execution mode")
	}
	if config.Mode == engine.ExecutionModeNative {
		binary := strings.TrimSpace(config.AgentctlBinary)
		if !filepath.IsAbs(binary) {
			return nil, errors.New("agentctl binary path must be absolute")
		}
		info, err := os.Stat(binary)
		if err != nil {
			return nil, fmt.Errorf("inspect agentctl binary: %w", err)
		}
		if info.IsDir() || info.Mode()&0o111 == 0 {
			return nil, errors.New("agentctl binary must be an executable file")
		}
		config.AgentctlBinary = binary
	}
	codexAuth := strings.TrimSpace(config.CodexAuthFile)
	if codexAuth == "" && config.Mode == engine.ExecutionModeNative {
		if home, err := os.UserHomeDir(); err == nil {
			candidate := filepath.Join(home, ".codex", "auth.json")
			if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() {
				codexAuth = candidate
			}
		}
	}
	if codexAuth != "" && (!filepath.IsAbs(codexAuth)) {
		return nil, errors.New("codex auth file path must be absolute")
	}
	config.CodexAuthFile = codexAuth
	config.ProviderEnv = append([]string(nil), config.ProviderEnv...)
	config.Images = defaultImages(config.Images)
	config.Environment = cloneEnvironment(config.Environment)
	return &AgentCtlExecutor{workspace: config.Repositories, engine: config.Engine, config: config}, nil
}

var (
	_ orchestration.AgentctlStarter        = (*AgentCtlExecutor)(nil)
	_ orchestration.AgentProcessController = (*AgentCtlExecutor)(nil)
)

func (e *AgentCtlExecutor) StartAgentctl(ctx context.Context, run model.RunConfig) (model.Process, error) {
	if e.engine == nil {
		return model.Process{}, errors.New("shared agentctl engine is not configured")
	}
	if err := run.Validate(); err != nil {
		return model.Process{}, err
	}
	if err := mountmodel.Validate(run.HostMounts); err != nil {
		return model.Process{}, fmt.Errorf("validate agent host mounts: %w", err)
	}
	useCodexBinding := e.config.Mode == engine.ExecutionModeContainer && run.Runtime == runtimecatalog.Codex && (e.config.CodexAuthFile != "" || e.config.CodexAuthBinding)
	requirements := engine.Requirements{
		Mode:               e.config.Mode,
		DurableWork:        true,
		DurableState:       true,
		CredentialBindings: useCodexBinding,
	}
	if len(run.HostMounts) != 0 {
		requirements.ExplicitHostBinds = true
	}
	if err := e.engine.Validate(ctx, requirements); err != nil {
		return model.Process{}, err
	}
	scope := engine.Scope{OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, TaskID: run.TaskID}
	workspace, err := e.engine.ResolveWorkspace(ctx, engine.WorkspaceRequest{
		Scope: scope, Intent: engine.WorkspaceEnsureForAgent,
		Required: engine.WorkspaceGuarantees{DurableWork: true, DurableState: true, PlatformSetup: true},
	})
	if err != nil {
		return model.Process{}, fmt.Errorf("resolve agent workspace: %w", err)
	}
	if !workspace.Exists || workspace.WorkPath == "" || workspace.StatePath == "" || workspace.PreparationDirectory == "" {
		return model.Process{}, fmt.Errorf("%w: engine returned incomplete agent workspace", engine.ErrInvalidSpec)
	}
	if err = e.workspace.Repositories.Prepare(ctx, workspace.PreparationDirectory, run.WorkspaceID, run.Repositories); err != nil {
		return model.Process{}, fmt.Errorf("prepare task repositories: %w", err)
	}
	artifact, err := e.selectArtifact(run)
	if err != nil {
		return model.Process{}, err
	}
	workPath, statePath, listenHost, listenPort := e.executionPaths(workspace)
	environment, environmentNames, hostPolicy := e.resolveRunEnvironment(run)
	arguments := typedArguments(run, workPath, statePath, environmentNames, e.config.Mode == engine.ExecutionModeNative)
	mounts := make([]engine.Mount, 0, len(run.HostMounts))
	for _, hostMount := range run.HostMounts {
		target := hostMount.Target
		if strings.HasPrefix(target, "~/") {
			relative := strings.TrimPrefix(target, "~/")
			target = path.Join(sessionpath.Home(statePath, run.OrganizationID, run.WorkspaceID, run.TaskID, run.TaskSessionID), relative)
		}
		mounts = append(mounts, engine.Mount{SourceKind: engine.MountSourceHost, Source: hostMount.Source, Target: target, ReadOnly: hostMount.ReadOnly})
	}
	credentials := []engine.CredentialBinding(nil)
	if useCodexBinding {
		credentials = append(credentials, engine.CredentialBinding{Name: "codex-auth", Target: codexContainerAuthPath, ReadOnly: true})
	}
	writableRoots := make([]string, 0, len(run.Repositories))
	for _, repository := range run.Repositories {
		relative, relErr := checkout.RepositoryRelativePath(repository.ProviderName, repository.ID)
		if relErr != nil {
			return model.Process{}, relErr
		}
		writableRoots = append(writableRoots, relative)
	}
	restart := engine.RestartNever
	if e.config.Mode == engine.ExecutionModeContainer && run.TaskSessionID != "" && e.engine.Capabilities().RestartModes {
		restart = engine.RestartOnFailure
	}
	spec := engine.LaunchSpec{
		Key:   engine.ResourceKey{Scope: scope, Class: engine.ResourceClassAgent, ExecutionID: run.RunID, Qualifier: string(run.Runtime)},
		Class: engine.ResourceClassAgent, Mode: e.config.Mode, Artifact: artifact,
		Arguments: arguments, Environment: environment, Workspace: workspace, Mounts: mounts,
		CredentialBindings: credentials,
		Sharing:            engine.WorkspaceSharing{ProtectedAncestry: e.config.Mode == engine.ExecutionModeContainer, WritableRoots: writableRoots},
		HostEnvironment:    hostPolicy,
		Endpoints:          []engine.Endpoint{{Name: "agentctl", Protocol: "http", ListenHost: listenHost, ListenPort: listenPort, ReadinessPath: "/health", Ingress: engine.IngressWorkerControl}},
		Requirements:       requirements,
		Restart:            restart,
	}
	resource, startErr := e.engine.Start(ctx, spec)
	process := model.Process{ID: resource.Ref.ConcreteID, Endpoint: resource.Endpoint}
	if startErr != nil {
		return process, fmt.Errorf("start agentctl workload: %w", startErr)
	}
	if process.ID == "" || process.Endpoint == "" {
		return process, fmt.Errorf("%w: engine returned incomplete agent resource", engine.ErrUncertain)
	}
	return process, nil
}

func (e *AgentCtlExecutor) AgentctlRunning(ctx context.Context, processID string) (bool, error) {
	if strings.TrimSpace(processID) == "" {
		return false, errors.New("agentctl process identity is required")
	}
	observation, err := e.engine.Inspect(ctx, engine.Lookup{Ref: engine.ResourceRef{
		Key: engine.ResourceKey{Class: engine.ResourceClassAgent}, ConcreteID: strings.TrimSpace(processID),
	}})
	if err != nil {
		return false, err
	}
	return observation.State == engine.ObservationActive, nil
}

func (e *AgentCtlExecutor) AgentctlRunningByRunID(ctx context.Context, runID string) (bool, error) {
	observation, err := e.inspectByRunID(ctx, runID)
	if err != nil {
		return false, err
	}
	return observation.State == engine.ObservationActive, nil
}

func (e *AgentCtlExecutor) TerminateAgentctl(ctx context.Context, processID string) error {
	ref := engine.ResourceRef{Key: engine.ResourceKey{Class: engine.ResourceClassAgent}, ConcreteID: strings.TrimSpace(processID)}
	if ref.ConcreteID == "" {
		return errors.New("agentctl process identity is required")
	}
	if err := e.engine.Stop(ctx, ref); err != nil {
		return err
	}
	return e.engine.Cleanup(ctx, ref)
}

func (e *AgentCtlExecutor) TerminateAgentctlByRunID(ctx context.Context, runID string) error {
	observation, err := e.inspectByRunID(ctx, runID)
	if err != nil || observation.State == engine.ObservationAbsent {
		return err
	}
	if observation.State == engine.ObservationActive {
		if err = e.engine.Stop(ctx, observation.Resource.Ref); err != nil {
			return err
		}
	}
	return e.engine.Cleanup(ctx, observation.Resource.Ref)
}

func (e *AgentCtlExecutor) inspectByRunID(ctx context.Context, runID string) (engine.Observation, error) {
	if strings.TrimSpace(runID) == "" {
		return engine.Observation{}, errors.New("agent run identity is required")
	}
	key := engine.ResourceKey{Class: engine.ResourceClassAgent, ExecutionID: runID}
	return e.engine.Inspect(ctx, engine.Lookup{Key: &key})
}

func (e *AgentCtlExecutor) selectArtifact(run model.RunConfig) (engine.Artifact, error) {
	if e.config.Mode == engine.ExecutionModeNative {
		return engine.Artifact{ExecutablePath: e.config.AgentctlBinary}, nil
	}
	image := strings.TrimSpace(run.ExecutionImage)
	if image == "" {
		image = strings.TrimSpace(e.config.Images[run.Runtime])
	}
	if image == "" {
		return engine.Artifact{}, fmt.Errorf("no container image configured for runtime %q", run.Runtime)
	}
	return engine.Artifact{ContainerImage: image}, nil
}

func (e *AgentCtlExecutor) executionPaths(workspace engine.Workspace) (workPath, statePath, listenHost string, listenPort int) {
	if e.config.Mode == engine.ExecutionModeNative {
		return workspace.WorkPath, workspace.StatePath, "127.0.0.1", 0
	}
	return "/workspace", "/agentctl-state", "0.0.0.0", 8081
}

func (e *AgentCtlExecutor) resolveRunEnvironment(run model.RunConfig) (map[string]string, []string, engine.HostEnvironmentPolicy) {
	allowed := append([]string(nil), e.config.ProviderEnv...)
	if e.config.Mode == engine.ExecutionModeNative {
		allowed = append(allowed, "PATH", "HOME", "TMPDIR", "TMP", "TEMP", "USER", "LOGNAME", "LANG", "LC_ALL", "SYSTEMROOT", "WINDIR", "APPDATA", "LOCALAPPDATA")
	}
	overrides := cloneEnvironment(e.config.Environment)
	if run.Runtime == runtimecatalog.Codex && (e.config.CodexAuthFile != "" || e.config.CodexAuthBinding) {
		if e.config.Mode == engine.ExecutionModeContainer {
			overrides[codexAuthEnvironment] = codexContainerAuthPath
		} else {
			overrides[codexAuthEnvironment] = e.config.CodexAuthFile
		}
	}
	resolved := ResolveEnvironment(os.Environ(), allowed, overrides, []string{codexAuthEnvironment})
	values := make(map[string]string, len(resolved))
	names := make([]string, 0, len(resolved))
	for _, entry := range resolved {
		name, value, _ := strings.Cut(entry, "=")
		values[name] = value
		names = append(names, name)
	}
	if e.config.Mode == engine.ExecutionModeNative && !containsName(names, "PATH") {
		names = append(names, "PATH")
	}
	return values, names, engine.HostEnvironmentPolicy{}
}

func typedArguments(run model.RunConfig, workPath, statePath string, environmentNames []string, native bool) []engine.Argument {
	arguments := []engine.Argument{
		{Kind: engine.ArgumentLiteral, Value: "-listen"},
		{Kind: engine.ArgumentEndpointAddress, Endpoint: "agentctl"},
		{Kind: engine.ArgumentLiteral, Value: "-runtime"},
		{Kind: engine.ArgumentLiteral, Value: string(run.Runtime)},
		{Kind: engine.ArgumentLiteral, Value: "-work-dir"},
		{Kind: engine.ArgumentLiteral, Value: workPath},
		{Kind: engine.ArgumentLiteral, Value: "-state-dir"},
		{Kind: engine.ArgumentLiteral, Value: statePath},
	}
	if native || run.TaskSessionID != "" {
		arguments = append(arguments, engine.Argument{Kind: engine.ArgumentLiteral, Value: "-run-id"}, engine.Argument{Kind: engine.ArgumentLiteral, Value: run.RunID})
	}
	for _, name := range environmentNames {
		arguments = append(arguments, engine.Argument{Kind: engine.ArgumentLiteral, Value: "-env"}, engine.Argument{Kind: engine.ArgumentLiteral, Value: name})
	}
	return arguments
}

func containsName(names []string, target string) bool {
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}

func hasMode(capabilities engine.Capabilities, mode engine.ExecutionMode) bool {
	for _, candidate := range capabilities.Modes {
		if candidate == mode {
			return true
		}
	}
	return false
}

func defaultImages(values map[runtimecatalog.Runtime]string) map[runtimecatalog.Runtime]string {
	result := make(map[runtimecatalog.Runtime]string, len(runtimecatalog.Providers()))
	for _, provider := range runtimecatalog.Providers() {
		result[provider.Runtime()] = provider.DefaultImage()
	}
	for key, value := range values {
		if strings.TrimSpace(value) != "" {
			result[key] = strings.TrimSpace(value)
		}
	}
	return result
}

func cloneEnvironment(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
