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
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	mountmodel "github.com/josemiguelmelo/orkest/internal/server/workspacemount/model"
)

const (
	codexAuthEnvironment   = "ORKEST_CODEX_AUTH_FILE"
	codexContainerAuthPath = "/agentctl-state/codex-auth/auth.json"
)

type ExecutorConfig struct {
	Engine           engine.InfrastructureEngine
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
	engine    engine.InfrastructureEngine
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
	if err := engine.ValidateCapabilities(e.engine.Capabilities(), requirements); err != nil {
		return model.Process{}, err
	}
	storageID := taskstorage.Identity(run.OrganizationID, run.WorkspaceID, run.TaskID)
	bindings, err := e.engine.ResolveStorage(ctx, engine.StorageRequest{Owner: run.RunID, Generation: run.RunID, Bindings: []engine.StorageBindingRequest{
		{Name: "work", StorageID: storageID, Subpath: "work", Durable: true, Create: true, PlatformIO: true},
		{Name: "agentctl-state", StorageID: storageID, Subpath: "agentctl-state", Durable: true, Create: true, PlatformIO: true},
	}})
	if err != nil {
		return model.Process{}, fmt.Errorf("resolve agent storage: %w", err)
	}
	if len(bindings) != 2 || bindings[0].HostPath == "" || bindings[1].HostPath == "" {
		return model.Process{}, fmt.Errorf("%w: engine returned incomplete agent storage bindings", engine.ErrInvalidSpec)
	}
	workspace := resolvedWorkspace{Exists: true, Identity: storageID, PreparationDirectory: bindings[0].HostPath, WorkPath: bindings[0].HostPath, StatePath: bindings[1].HostPath}
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
	environment, environmentNames := e.resolveRunEnvironment(run)
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
		credentials = append(credentials, engine.CredentialBinding{Name: "codex-auth", Target: codexContainerAuthPath, ReadOnly: true, EnvironmentName: codexAuthEnvironment})
	}
	restart := engine.RestartNever
	if e.config.Mode == engine.ExecutionModeContainer && run.TaskSessionID != "" && e.engine.Capabilities().RestartModes {
		restart = engine.RestartOnFailure
	}
	workTarget, stateTarget := workPath, statePath
	if e.config.Mode == engine.ExecutionModeContainer {
		workTarget, stateTarget = "/workspace", "/agentctl-state"
	}
	workload := engine.Workload{
		ID: "agentctl", Mode: e.config.Mode, Artifact: artifact,
		Endpoints: []engine.Endpoint{{Name: "agentctl", Protocol: "http", ListenHost: listenHost, ListenPort: listenPort, Exposure: engine.ExposurePlatform}},
		Arguments: arguments, Environment: environment, WorkingPath: workTarget,
		Mounts: mounts, Credentials: credentials, Restart: restart,
	}
	return e.startInfrastructure(ctx, run, workload, storageID, workspace.WorkPath, statePath, stateTarget)
}

type resolvedWorkspace struct {
	Exists               bool
	Identity             string
	PreparationDirectory string
	WorkPath             string
	StatePath            string
}

func (e *AgentCtlExecutor) startInfrastructure(ctx context.Context, run model.RunConfig, workload engine.Workload, storageID, hostWorkPath, statePath, stateTarget string) (model.Process, error) {
	containerMode := workload.Mode == engine.ExecutionModeContainer
	workTarget := workload.WorkingPath
	workReadOnly := containerMode
	storage := []engine.StorageAttachment{
		{WorkloadID: workload.ID, Binding: "work", StorageID: storageID, Subpath: "work", Target: workTarget, ReadOnly: workReadOnly, Create: true, Durable: true},
		{WorkloadID: workload.ID, Binding: "agentctl-state", StorageID: storageID, Subpath: "agentctl-state", Target: stateTarget, Create: true, Durable: true},
	}
	if containerMode {
		if len(run.Repositories) > 0 {
			resolvedWorkPath, err := filepath.EvalSymlinks(hostWorkPath)
			if err != nil {
				return model.Process{}, fmt.Errorf("resolve task work storage before mount planning: %w", err)
			}
			repositoryParent := filepath.Join(resolvedWorkPath, "repositories")
			if err := os.MkdirAll(repositoryParent, 0o700); err != nil {
				return model.Process{}, fmt.Errorf("prepare protected repository mount parent: %w", err)
			}
			resolvedParent, err := filepath.EvalSymlinks(repositoryParent)
			if err != nil || resolvedParent != filepath.Join(resolvedWorkPath, "repositories") {
				return model.Process{}, fmt.Errorf("%w: repository mount parent has unstable ancestry", engine.ErrInvalidSpec)
			}
			workload.Mounts = append(workload.Mounts, engine.Mount{SourceKind: engine.MountSourceStorage, Source: repositoryParent, Target: "/workspace/repositories", ReadOnly: true})
			for _, repository := range run.Repositories {
				relative, relErr := checkout.RepositoryRelativePath(repository.ProviderName, repository.ID)
				if relErr != nil {
					return model.Process{}, relErr
				}
				source := filepath.Join(resolvedWorkPath, filepath.FromSlash(relative))
				resolvedSource, sourceErr := filepath.EvalSymlinks(source)
				if sourceErr != nil || resolvedSource != source || !pathIsWithin(repositoryParent, source) {
					return model.Process{}, fmt.Errorf("%w: repository checkout has unstable mount ancestry", engine.ErrInvalidSpec)
				}
				workload.Mounts = append(workload.Mounts, engine.Mount{SourceKind: engine.MountSourceStorage, Source: source, Target: path.Join("/workspace", relative)})
			}
		}
	}
	plan := engine.WorkloadPlan{
		Group:     engine.GroupIdentity{Owner: run.RunID, Generation: run.RunID},
		Workloads: []engine.Workload{workload},
		Storage:   storage,
	}
	group, startErr := e.engine.StartPlan(ctx, plan)
	process := model.Process{}
	if group.Ref.Identity.Owner != "" && group.Ref.Provider != "" {
		process.ID, _ = engine.EncodeGroupRef(group.Ref)
	}
	process.Endpoint = group.Endpoints["agentctl"]
	if startErr != nil {
		return process, fmt.Errorf("start agentctl infrastructure plan: %w", startErr)
	}
	if process.ID == "" || process.Endpoint == "" {
		return process, fmt.Errorf("%w: engine returned an incomplete agent workload group", engine.ErrUncertain)
	}
	return process, nil
}

func pathIsWithin(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (e *AgentCtlExecutor) AgentctlRunning(ctx context.Context, processID string) (bool, error) {
	if strings.TrimSpace(processID) == "" {
		return false, errors.New("agentctl process identity is required")
	}
	ref, err := engine.DecodeGroupRef(strings.TrimSpace(processID))
	if err != nil {
		return false, err
	}
	observation, err := e.engine.InspectGroup(ctx, ref)
	if err != nil {
		return false, err
	}
	if observation.State == engine.GroupUncertain {
		return false, engine.ErrUncertain
	}
	return observation.State == engine.GroupActive, nil
}

func (e *AgentCtlExecutor) AgentctlRunningByRunID(ctx context.Context, runID string) (bool, error) {
	observation, found, err := e.inspectInfrastructureByRunID(ctx, runID)
	if err != nil {
		return false, err
	}
	if found {
		if observation.State == engine.GroupUncertain {
			return false, engine.ErrUncertain
		}
		return observation.State == engine.GroupActive, nil
	}
	return false, nil
}

func (e *AgentCtlExecutor) TerminateAgentctl(ctx context.Context, processID string) error {
	ref, err := engine.DecodeGroupRef(strings.TrimSpace(processID))
	if err != nil {
		return errors.New("agentctl process identity is invalid")
	}
	if err := e.engine.StopGroup(ctx, ref); err != nil {
		return err
	}
	return e.engine.CleanupGroup(ctx, ref)
}

func (e *AgentCtlExecutor) TerminateAgentctlByRunID(ctx context.Context, runID string) error {
	observation, found, err := e.inspectInfrastructureByRunID(ctx, runID)
	if err != nil || !found || observation.State == engine.GroupAbsent {
		return err
	}
	if observation.State == engine.GroupUncertain {
		return engine.ErrUncertain
	}
	if observation.State == engine.GroupActive {
		if err = e.engine.StopGroup(ctx, observation.Group.Ref); err != nil {
			return err
		}
	}
	return e.engine.CleanupGroup(ctx, observation.Group.Ref)
}

func (e *AgentCtlExecutor) inspectInfrastructureByRunID(ctx context.Context, runID string) (engine.GroupObservation, bool, error) {
	if strings.TrimSpace(runID) == "" {
		return engine.GroupObservation{}, false, errors.New("agent run identity is required")
	}
	groups, err := e.engine.ListGroups(ctx, engine.Selector{GroupOwner: strings.TrimSpace(runID)})
	if err != nil {
		return engine.GroupObservation{}, false, err
	}
	var match *engine.ResourceGroup
	for index := range groups {
		if groups[index].Ref.Identity.Generation != strings.TrimSpace(runID) {
			continue
		}
		if match != nil {
			return engine.GroupObservation{}, false, engine.ErrUncertain
		}
		match = &groups[index]
	}
	if match == nil {
		return engine.GroupObservation{}, false, nil
	}
	observation, err := e.engine.InspectGroup(ctx, match.Ref)
	return observation, true, err
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

func (e *AgentCtlExecutor) executionPaths(workspace resolvedWorkspace) (workPath, statePath, listenHost string, listenPort int) {
	if e.config.Mode == engine.ExecutionModeNative {
		return workspace.WorkPath, workspace.StatePath, "127.0.0.1", 0
	}
	return "/workspace", "/agentctl-state", "0.0.0.0", 8081
}

func (e *AgentCtlExecutor) resolveRunEnvironment(run model.RunConfig) (map[string]string, []string) {
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
	return values, names
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
