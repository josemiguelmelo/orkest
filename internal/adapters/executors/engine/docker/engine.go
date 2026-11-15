// Package docker implements container infrastructure for shared executors.
package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/containerd/errdefs"
	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	runtimecatalog "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/platform/agentctllogs"
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	client "github.com/moby/moby/client"
)

const (
	workMountPath            = "/workspace"
	stateMountPath           = "/agentctl-state"
	codexAuthMountPath       = stateMountPath + "/codex-auth/auth.json"
	codexAuthFileEnvironment = "ORKEST_CODEX_AUTH_FILE"
	checkoutMountLayoutLabel = "orkest.checkout_mount_layout"
	checkoutMountLayout      = "protected-v1"
)

type AgentClient interface {
	ContainerCreate(context.Context, client.ContainerCreateOptions) (client.ContainerCreateResult, error)
	ContainerStart(context.Context, string, client.ContainerStartOptions) (client.ContainerStartResult, error)
	ContainerStop(context.Context, string, client.ContainerStopOptions) (client.ContainerStopResult, error)
	ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerRemove(context.Context, string, client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
	ContainerLogs(context.Context, string, client.ContainerLogsOptions) (client.ContainerLogsResult, error)
	Close() error
}

type EditorClient interface {
	ContainerCreate(context.Context, client.ContainerCreateOptions) (client.ContainerCreateResult, error)
	ContainerStart(context.Context, string, client.ContainerStartOptions) (client.ContainerStartResult, error)
	ContainerStop(context.Context, string, client.ContainerStopOptions) (client.ContainerStopResult, error)
	ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerRemove(context.Context, string, client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
	ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error)
	NetworkCreate(context.Context, string, client.NetworkCreateOptions) (client.NetworkCreateResult, error)
	NetworkInspect(context.Context, string, client.NetworkInspectOptions) (client.NetworkInspectResult, error)
	NetworkConnect(context.Context, string, client.NetworkConnectOptions) (client.NetworkConnectResult, error)
	NetworkDisconnect(context.Context, string, client.NetworkDisconnectOptions) (client.NetworkDisconnectResult, error)
	NetworkRemove(context.Context, string, client.NetworkRemoveOptions) (client.NetworkRemoveResult, error)
	NetworkList(context.Context, client.NetworkListOptions) (client.NetworkListResult, error)
}

type Client interface {
	AgentClient
	EditorClient
}

type Config struct {
	DataRoot      string
	NetworkName   string
	ContainerUser string
	CodexAuthFile string
	AgentctlPort  int
	Client        AgentClient
}

type ExecutorEngine struct {
	client       AgentClient
	editorClient EditorClient
	config       Config
	installation string
	closeOnce    sync.Once
	closeErr     error
	editorMu     sync.Mutex
}

var _ engine.ExecutorEngine = (*ExecutorEngine)(nil)

// NewClient constructs the shared Docker API client used by agent and editor
// engines. The factory owns and closes it through ExecutorEngine.
func NewClient() (Client, error) { return client.New(client.FromEnv) }

func New(config Config) (*ExecutorEngine, error) {
	if config.Client == nil {
		return nil, errors.New("docker engine client is required")
	}
	root := strings.TrimSpace(config.DataRoot)
	if root == "" {
		return nil, errors.New("docker engine data root is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve Docker engine data root: %w", err)
	}
	config.DataRoot = root
	config.NetworkName = strings.TrimSpace(config.NetworkName)
	config.CodexAuthFile = strings.TrimSpace(config.CodexAuthFile)
	if config.CodexAuthFile != "" && !filepath.IsAbs(config.CodexAuthFile) {
		return nil, errors.New("codex auth file path must be absolute")
	}
	if config.AgentctlPort == 0 {
		config.AgentctlPort = 8081
	}
	if config.AgentctlPort < 1 || config.AgentctlPort > 65535 {
		return nil, errors.New("agentctl container port must be between 1 and 65535")
	}
	if config.ContainerUser == "" {
		current, userErr := user.Current()
		if userErr != nil {
			return nil, fmt.Errorf("resolve Docker container file ownership: %w", userErr)
		}
		config.ContainerUser = current.Uid + ":" + current.Gid
	}
	uid, gid, ok := strings.Cut(config.ContainerUser, ":")
	if !ok {
		return nil, errors.New("docker container user must use uid:gid format")
	}
	if _, err := strconv.ParseUint(uid, 10, 32); err != nil {
		return nil, errors.New("docker container user must use numeric uid:gid format")
	}
	if _, err := strconv.ParseUint(gid, 10, 32); err != nil {
		return nil, errors.New("docker container user must use numeric uid:gid format")
	}
	hash := sha256.Sum256([]byte(root))
	engine := &ExecutorEngine{client: config.Client, config: config, installation: hex.EncodeToString(hash[:])}
	if editorClient, ok := config.Client.(EditorClient); ok {
		engine.editorClient = editorClient
	}
	return engine, nil
}

func (e *ExecutorEngine) Capabilities() engine.Capabilities {
	return engine.Capabilities{
		Modes:       []engine.ExecutionMode{engine.ExecutionModeContainer},
		DurableWork: true, DurableState: true, PlatformWorkspaceSetup: true, ReadOnlyRoot: true, ReadOnlyMounts: true,
		TemporaryMounts: true, ProtectedWorkspaceSharing: true,
		ConfirmedProcessTreeStop: true,
		ExplicitHostBinds:        true, CredentialBindings: e.config.CodexAuthFile != "", RestartModes: true,
		GatewayOnlyIngress:          e.editorClient != nil,
		DeniedEgress:                e.editorClient != nil,
		PrivateHTTPWebSocketRouting: e.editorClient != nil,
	}
}

func (e *ExecutorEngine) Validate(ctx context.Context, requirements engine.Requirements) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := engine.ValidateCapabilities(e.Capabilities(), requirements); err != nil {
		return err
	}
	if pinger, ok := e.client.(interface {
		Ping(context.Context, client.PingOptions) (client.PingResult, error)
	}); ok {
		if _, err := pinger.Ping(ctx, client.PingOptions{}); err != nil {
			return fmt.Errorf("validate Docker Engine availability: %w", err)
		}
	}
	if requirements.CredentialBindings && e.config.CodexAuthFile != "" {
		info, err := os.Stat(e.config.CodexAuthFile)
		if err != nil {
			return fmt.Errorf("inspect configured Docker credential binding: %w", err)
		}
		if !info.Mode().IsRegular() {
			return errors.New("configured Docker credential binding is not a regular file")
		}
	}
	return nil
}

func (e *ExecutorEngine) ResolveWorkspace(ctx context.Context, request engine.WorkspaceRequest) (engine.Workspace, error) {
	if err := ctx.Err(); err != nil {
		return engine.Workspace{}, err
	}
	if request.Scope.OrganizationID == "" || request.Scope.WorkspaceID == "" || request.Scope.TaskID == "" {
		return engine.Workspace{}, fmt.Errorf("%w: task workspace scope is required", engine.ErrInvalidSpec)
	}
	if err := engine.ValidateWorkspaceGuarantees(e.Capabilities(), request.Required); err != nil {
		return engine.Workspace{}, err
	}
	work, state := taskstorage.Directories(e.config.DataRoot, request.Scope.OrganizationID, request.Scope.WorkspaceID, request.Scope.TaskID)
	workspace := engine.Workspace{Identity: filepath.Base(filepath.Dir(work)), WorkPath: work, StatePath: state, PreparationDirectory: work, CompatibilityID: filepath.Base(filepath.Dir(work))}
	if request.Intent == engine.WorkspaceInspect {
		paths := []string{work}
		if request.Required.DurableState {
			paths = append(paths, state)
		}
		for _, path := range paths {
			info, err := os.Lstat(path)
			if errors.Is(err, os.ErrNotExist) {
				return workspace, nil
			}
			if err != nil {
				return engine.Workspace{}, fmt.Errorf("inspect Docker task workspace: %w", err)
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return engine.Workspace{}, fmt.Errorf("%w: Docker task workspace path is not a regular directory", engine.ErrInvalidSpec)
			}
		}
		workspace.Exists = true
		return workspace, nil
	}
	if request.Intent != engine.WorkspaceEnsureForAgent {
		return engine.Workspace{}, fmt.Errorf("%w: unknown workspace intent", engine.ErrInvalidSpec)
	}
	for _, path := range []string{work, state} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return engine.Workspace{}, fmt.Errorf("create Docker task directory: %w", err)
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return engine.Workspace{}, fmt.Errorf("restrict Docker task directory permissions: %w", err)
		}
	}
	workspace.Exists = true
	return workspace, nil
}

func (e *ExecutorEngine) Start(ctx context.Context, spec engine.LaunchSpec) (engine.Resource, error) {
	resource := engine.Resource{Ref: engine.ResourceRef{Key: spec.Key}}
	if err := engine.ValidateSpec(spec); err != nil {
		return resource, err
	}
	if spec.Mode != engine.ExecutionModeContainer {
		return resource, engine.Unsupported("Docker engine supports container workloads only")
	}
	if spec.Class == engine.ResourceClassEditor {
		return e.startEditor(ctx, spec)
	}
	if spec.Class != engine.ResourceClassAgent {
		return resource, engine.Unsupported("Docker engine resource class")
	}
	if spec.Key.InstallationID != "" && spec.Key.InstallationID != e.installation {
		return resource, engine.ErrIdentityMismatch
	}
	if err := e.Validate(ctx, spec.Requirements); err != nil {
		return resource, err
	}
	workspace, err := e.ResolveWorkspace(ctx, engine.WorkspaceRequest{Scope: spec.Key.Scope, Intent: engine.WorkspaceInspect, Required: engine.WorkspaceGuarantees{DurableWork: true, DurableState: true}})
	if err != nil {
		return resource, err
	}
	if !workspace.Exists || workspace.Identity != spec.Workspace.Identity || workspace.WorkPath != spec.Workspace.WorkPath || workspace.StatePath != spec.Workspace.StatePath {
		return resource, fmt.Errorf("%w: Docker task workspace changed before launch", engine.ErrIdentityMismatch)
	}
	runtimeName := spec.Key.Qualifier
	if runtimeName == "" {
		return resource, fmt.Errorf("%w: runtime qualifier is required for legacy Docker naming", engine.ErrInvalidSpec)
	}
	name := containerName(runtimeName, spec.Key.ExecutionID)
	labels := labelsFor(spec.Key, runtimeName, e.installation)
	port := network.MustParsePort(fmt.Sprintf("%d/tcp", e.config.AgentctlPort))
	arguments, err := resolveArguments(spec, e.config.AgentctlPort)
	if err != nil {
		return resource, err
	}
	environment := make([]string, 0, len(spec.Environment))
	environmentNames := make([]string, 0, len(spec.Environment))
	for key, value := range spec.Environment {
		environment = append(environment, key+"="+value)
		environmentNames = append(environmentNames, key)
	}
	// Stable environment ordering keeps launch diffs and diagnostics predictable.
	sort.Strings(environment)
	sort.Strings(environmentNames)
	mounts := []mount.Mount{
		{Type: mount.TypeBind, Source: workspace.WorkPath, Target: workMountPath},
		{Type: mount.TypeBind, Source: workspace.StatePath, Target: stateMountPath},
	}
	for _, item := range spec.Mounts {
		if item.SourceKind != engine.MountSourceHost {
			return resource, engine.Unsupported("Docker agent mount source is not a host path")
		}
		if err := e.prepareHostMountTarget(workspace.StatePath, item.Target); err != nil {
			return resource, err
		}
		mounts = append(mounts, mount.Mount{Type: mount.TypeBind, Source: item.Source, Target: item.Target, ReadOnly: item.ReadOnly})
	}
	if len(spec.GeneratedFiles) != 0 || spec.HostEnvironment.InheritAllowlisted {
		return resource, engine.Unsupported("Docker engine does not materialize generated files or inherit host environment")
	}
	for _, binding := range spec.CredentialBindings {
		if binding.Name != "codex-auth" || e.config.CodexAuthFile == "" || binding.Target != codexAuthMountPath {
			return resource, engine.Unsupported("Docker credential binding is unavailable")
		}
		if err = ensureCodexMountTarget(workspace.StatePath); err != nil {
			return resource, err
		}
		mounts = append(mounts, mount.Mount{Type: mount.TypeBind, Source: e.config.CodexAuthFile, Target: binding.Target, ReadOnly: true})
	}
	checkoutMounts, err := prepareProtectedCheckoutMounts(workspace.WorkPath, spec.Sharing.WritableRoots)
	if err != nil {
		return resource, err
	}
	mounts = append(mounts, checkoutMounts...)
	portBindings := network.PortMap{port: {{HostIP: netip.MustParseAddr("127.0.0.1")}}}
	networkMode := container.NetworkMode("")
	if e.config.NetworkName != "" {
		networkMode = container.NetworkMode(e.config.NetworkName)
		portBindings = nil
	}
	restart := container.RestartPolicyDisabled
	if spec.Restart == engine.RestartOnFailure {
		restart = container.RestartPolicyUnlessStopped
	}
	userName, err := e.containerUser(spec.Security)
	if err != nil {
		return resource, err
	}
	tmpfs := make(map[string]string, len(spec.TemporaryMounts))
	for _, temporary := range spec.TemporaryMounts {
		tmpfs[temporary.Target] = tmpfsOptions(temporary)
	}
	resources := container.Resources{Memory: spec.Limits.MemoryBytes, NanoCPUs: spec.Limits.NanoCPUs}
	if spec.Limits.ProcessCount > 0 {
		processes := spec.Limits.ProcessCount
		resources.PidsLimit = &processes
	}
	capabilities := []string(nil)
	if spec.Security.DropAllCapabilities {
		capabilities = []string{"ALL"}
	}
	securityOptions := []string(nil)
	if spec.Security.NoPrivilegeEscalation {
		securityOptions = []string{"no-new-privileges:true"}
	}
	command := arguments
	if len(command) == 0 {
		command = spec.Artifact.ContainerCommand
	}
	created, err := e.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     &container.Config{Image: spec.Artifact.ContainerImage, User: userName, Entrypoint: spec.Artifact.ContainerEntrypoint, Env: environment, Cmd: command, ExposedPorts: network.PortSet{port: {}}, Labels: labels},
		HostConfig: &container.HostConfig{Mounts: mounts, PortBindings: portBindings, NetworkMode: networkMode, RestartPolicy: container.RestartPolicy{Name: restart}, ReadonlyRootfs: spec.Security.ReadOnlyRoot, CapDrop: capabilities, SecurityOpt: securityOptions, Tmpfs: tmpfs, Resources: resources},
		Name:       name,
	})
	if err != nil {
		resource.Ref.ConcreteID = name
		return resource, fmt.Errorf("create agentctl container: %w", err)
	}
	resource.Ref.ConcreteID = created.ID
	if err = ctx.Err(); err != nil {
		return resource, e.removeAfterFailure(ctx, created.ID, fmt.Errorf("start agentctl container: %w", err))
	}
	_, startErr := e.client.ContainerStart(ctx, created.ID, client.ContainerStartOptions{})
	inspection, inspectErr := e.client.ContainerInspect(ctx, created.ID, client.ContainerInspectOptions{})
	if inspectErr != nil {
		if startErr != nil {
			return resource, errors.Join(fmt.Errorf("start agentctl container: %w", startErr), fmt.Errorf("inspect uncertain container state: %w", inspectErr))
		}
		return resource, fmt.Errorf("inspect started agentctl container: %w", inspectErr)
	}
	if inspection.Container.State == nil {
		failure := errors.New("docker did not report agentctl container state")
		if startErr != nil {
			failure = errors.Join(fmt.Errorf("start agentctl container: %w", startErr), failure)
		}
		return resource, failure
	}
	if !inspection.Container.State.Running {
		failure := startErr
		if failure == nil {
			failure = errors.New("agentctl container exited before becoming reachable")
		}
		return resource, e.removeAfterFailure(ctx, created.ID, fmt.Errorf("start agentctl container: %w", failure))
	}
	resource.State = engine.ResourceRunning
	if e.config.NetworkName != "" {
		resource.Endpoint = fmt.Sprintf("http://%s:%d", name, e.config.AgentctlPort)
	} else {
		resource.Endpoint, err = publishedEndpoint(inspection.Container, e.config.AgentctlPort)
		if err != nil {
			return resource, e.removeAfterFailure(ctx, created.ID, err)
		}
	}
	go e.forwardLogs(created.ID, spec.Key.ExecutionID)
	return resource, nil
}

func (e *ExecutorEngine) Inspect(ctx context.Context, lookup engine.Lookup) (engine.Observation, error) {
	key := lookup.Ref.Key
	if lookup.Key != nil {
		key = *lookup.Key
	}
	if key.InstallationID != "" && key.InstallationID != e.installation {
		return engine.Observation{}, engine.ErrIdentityMismatch
	}
	if key.Class == engine.ResourceClassEditor {
		return e.inspectEditor(ctx, lookup, key)
	}
	if lookup.Ref.ConcreteID != "" {
		observation, err := e.inspectID(ctx, lookup.Ref.ConcreteID, key)
		if errors.Is(err, errNotFound) {
			return engine.Observation{State: engine.ObservationAbsent}, nil
		}
		return observation, err
	}
	if key.ExecutionID == "" {
		return engine.Observation{}, fmt.Errorf("%w: Docker lookup needs an execution ID or concrete ID", engine.ErrInvalidSpec)
	}
	names := containerNamesForRun(key.ExecutionID)
	for index, name := range names {
		expected := key
		if index == len(names)-1 {
			expected.Qualifier = ""
		}
		observation, err := e.inspectID(ctx, name, expected)
		if err != nil {
			if errors.Is(err, errNotFound) {
				continue
			}
			return engine.Observation{}, err
		}
		if observation.State != engine.ObservationAbsent {
			return observation, nil
		}
	}
	return engine.Observation{State: engine.ObservationAbsent}, nil
}

var errNotFound = errors.New("docker resource not found")

func (e *ExecutorEngine) inspectID(ctx context.Context, id string, expected engine.ResourceKey) (engine.Observation, error) {
	result, err := e.client.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return engine.Observation{State: engine.ObservationAbsent}, errNotFound
		}
		return engine.Observation{}, fmt.Errorf("inspect Docker workload: %w", err)
	}
	labels := map[string]string{}
	if result.Container.Config != nil {
		labels = result.Container.Config.Labels
	}
	if labels["orkest.managed"] != "agentctl" {
		return engine.Observation{}, engine.ErrIdentityMismatch
	}
	if installation := labels["orkest.installation"]; installation != "" && installation != e.installation {
		return engine.Observation{}, engine.ErrIdentityMismatch
	}
	if expected.ExecutionID != "" {
		runHash, _ := hashScope(expected.ExecutionID)
		if labels["orkest.run_id_hash"] != runHash {
			return engine.Observation{}, engine.ErrIdentityMismatch
		}
	}
	if expected.Scope.OrganizationID != "" && !matchesScope(labels, expected.Scope) {
		return engine.Observation{}, engine.ErrIdentityMismatch
	}
	if expected.Qualifier != "" && labels["orkest.runtime"] != expected.Qualifier {
		return engine.Observation{}, engine.ErrIdentityMismatch
	}
	if result.Container.State == nil {
		return engine.Observation{}, engine.ErrUncertain
	}
	state := engine.ObservationStopped
	resourceState := engine.ResourceStopped
	if result.Container.State.Running || result.Container.State.Restarting {
		state, resourceState = engine.ObservationActive, engine.ResourceRunning
	}
	resource := engine.Resource{Ref: engine.ResourceRef{Key: keyFromLabels(e.installation, labels), ConcreteID: result.Container.ID}, State: resourceState}
	if expected.ExecutionID != "" {
		resource.Ref.Key.ExecutionID = expected.ExecutionID
	}
	if expected.Scope.OrganizationID != "" {
		resource.Ref.Key.Scope = expected.Scope
	}
	if expected.Qualifier != "" {
		resource.Ref.Key.Qualifier = expected.Qualifier
	}
	if state == engine.ObservationActive {
		if e.config.NetworkName != "" {
			resource.Endpoint = fmt.Sprintf("http://%s:%d", strings.TrimPrefix(result.Container.Name, "/"), e.config.AgentctlPort)
		} else {
			resource.Endpoint, err = publishedEndpoint(result.Container, e.config.AgentctlPort)
			if err != nil {
				return engine.Observation{}, err
			}
		}
	}
	return engine.Observation{State: state, Resource: resource}, nil
}

func (e *ExecutorEngine) List(ctx context.Context, selector engine.Selector) ([]engine.Resource, error) {
	if selector.InstallationID != "" && selector.InstallationID != e.installation {
		return nil, engine.ErrIdentityMismatch
	}
	if selector.Class != engine.ResourceClassAgent {
		if selector.Class == engine.ResourceClassEditor {
			return e.listEditors(ctx, selector)
		}
		return nil, fmt.Errorf("%w: Docker list requires a supported resource class", engine.ErrInvalidSpec)
	}
	filters := client.Filters{}.Add("label", "orkest.managed=agentctl")
	lister, ok := e.client.(interface {
		ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error)
	})
	if !ok {
		return nil, engine.Unsupported("Docker client container listing")
	}
	listed, err := lister.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: filters})
	if err != nil {
		return nil, fmt.Errorf("list Docker agent workloads: %w", err)
	}
	resources := make([]engine.Resource, 0, len(listed.Items))
	for _, item := range listed.Items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if installation := item.Labels["orkest.installation"]; installation != "" && installation != e.installation {
			continue
		}
		observation, inspectErr := e.inspectID(ctx, item.ID, engine.ResourceKey{})
		if inspectErr != nil {
			if errors.Is(inspectErr, errNotFound) {
				continue
			}
			return nil, inspectErr
		}
		if observation.State == engine.ObservationAbsent {
			continue
		}
		if selector.Scope != nil && !scopeMatches(observation.Resource.Ref.Key.Scope, *selector.Scope) {
			continue
		}
		resources = append(resources, observation.Resource)
	}
	return resources, nil
}

func (e *ExecutorEngine) Stop(ctx context.Context, ref engine.ResourceRef) error {
	if ref.Key.Class == engine.ResourceClassEditor {
		return e.stopEditor(ctx, ref)
	}
	observation, err := e.Inspect(ctx, engine.Lookup{Ref: ref})
	if err != nil {
		if errors.Is(err, errNotFound) {
			return nil
		}
		return err
	}
	if observation.State != engine.ObservationActive {
		return nil
	}
	timeout := 10
	if _, err = e.client.ContainerStop(ctx, observation.Resource.Ref.ConcreteID, client.ContainerStopOptions{Timeout: &timeout}); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("stop Docker agent workload: %w", err)
	}
	confirmed, err := e.inspectID(ctx, observation.Resource.Ref.ConcreteID, ref.Key)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return nil
		}
		return err
	}
	if confirmed.State == engine.ObservationActive {
		return engine.ErrUncertain
	}
	return nil
}

func (e *ExecutorEngine) Cleanup(ctx context.Context, ref engine.ResourceRef) error {
	if ref.Key.Class == engine.ResourceClassEditor {
		return e.cleanupEditor(ctx, ref)
	}
	if ref.ConcreteID == "" {
		return fmt.Errorf("%w: Docker cleanup needs a concrete resource ID", engine.ErrInvalidSpec)
	}
	observation, err := e.inspectID(ctx, ref.ConcreteID, ref.Key)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return nil
		}
		return err
	}
	if observation.State == engine.ObservationActive {
		return errors.New("refusing to remove active Docker workload")
	}
	if _, err = e.client.ContainerRemove(ctx, observation.Resource.Ref.ConcreteID, client.ContainerRemoveOptions{}); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("remove stopped Docker workload: %w", err)
	}
	return nil
}

func (e *ExecutorEngine) Close() error {
	e.closeOnce.Do(func() { e.closeErr = e.client.Close() })
	return e.closeErr
}

func (e *ExecutorEngine) forwardLogs(id, runID string) {
	logs, err := e.client.ContainerLogs(context.Background(), id, client.ContainerLogsOptions{ShowStderr: true, Follow: true})
	if err != nil {
		return
	}
	defer func() { _ = logs.Close() }()
	reader, writer := io.Pipe()
	go func() {
		_, copyErr := stdcopy.StdCopy(io.Discard, writer, logs)
		_ = writer.CloseWithError(copyErr)
	}()
	agentctllogs.Forward(reader, "run_id", runID)
}

func (e *ExecutorEngine) removeAfterFailure(ctx context.Context, id string, cause error) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if _, err := e.client.ContainerRemove(cleanupCtx, id, client.ContainerRemoveOptions{Force: true}); err != nil {
		return errors.Join(cause, fmt.Errorf("remove incomplete Docker agent container: %w", err))
	}
	return cause
}

func (e *ExecutorEngine) prepareHostMountTarget(stateDir, target string) error {
	if !strings.HasPrefix(target, stateMountPath+"/") {
		return nil
	}
	relative, err := filepath.Rel(stateMountPath, filepath.FromSlash(target))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: task-state mount target escapes its workspace", engine.ErrInvalidSpec)
	}
	localTarget := filepath.Join(stateDir, relative)
	parent := filepath.Dir(localTarget)
	if err := prepareMountParent(stateDir, parent); err != nil {
		return err
	}
	info, err := os.Lstat(localTarget)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect host mount target: %w", err)
	}
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return errors.New("host mount target must not be a symlink")
	}
	return nil
}

func (e *ExecutorEngine) containerUser(requirements engine.SecurityRequirements) (string, error) {
	if requirements.RunAsUser == 0 && requirements.RunAsGroup == 0 {
		return e.config.ContainerUser, nil
	}
	uidText, gidText, _ := strings.Cut(e.config.ContainerUser, ":")
	uid, _ := strconv.ParseUint(uidText, 10, 32)
	gid, _ := strconv.ParseUint(gidText, 10, 32)
	if requirements.RunAsUser != 0 {
		uid = uint64(requirements.RunAsUser)
	}
	if requirements.RunAsGroup != 0 {
		gid = uint64(requirements.RunAsGroup)
	}
	if uid == 0 || gid == 0 {
		return "", engine.Unsupported("Docker engine requires a non-root editor identity")
	}
	return fmt.Sprintf("%d:%d", uid, gid), nil
}

func tmpfsOptions(mount engine.TemporaryMount) string {
	options := []string{"rw"}
	if mount.SizeBytes > 0 {
		options = append(options, fmt.Sprintf("size=%d", mount.SizeBytes))
	}
	if mount.Mode > 0 {
		options = append(options, fmt.Sprintf("mode=%o", mount.Mode))
	}
	if mount.NoSUID {
		options = append(options, "nosuid")
	}
	if mount.NoDevice {
		options = append(options, "nodev")
	}
	if mount.NoExecute {
		options = append(options, "noexec")
	}
	return strings.Join(options, ",")
}

func prepareMountParent(stateDir, parent string) error {
	relative, err := filepath.Rel(stateDir, parent)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("host mount parent must remain within task state")
	}
	current := stateDir
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "." || component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if err := os.Mkdir(current, 0o700); err != nil && !os.IsExist(err) {
				return fmt.Errorf("create host mount parent: %w", err)
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return fmt.Errorf("inspect host mount parent: %w", err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("host mount parent must be a directory without symlinks")
		}
	}
	return nil
}

func resolveArguments(spec engine.LaunchSpec, port int) ([]string, error) {
	values := make([]string, 0, len(spec.Arguments))
	for _, argument := range spec.Arguments {
		if argument.Kind == engine.ArgumentLiteral {
			values = append(values, argument.Value)
			continue
		}
		var found *engine.Endpoint
		for i := range spec.Endpoints {
			if spec.Endpoints[i].Name == argument.Endpoint {
				found = &spec.Endpoints[i]
				break
			}
		}
		if found == nil || found.Protocol != "http" || found.ListenPort != port || found.ListenHost != "0.0.0.0" || found.Ingress != engine.IngressWorkerControl {
			return nil, fmt.Errorf("%w: unsupported Docker endpoint binding", engine.ErrInvalidSpec)
		}
		values = append(values, fmt.Sprintf("0.0.0.0:%d", port))
	}
	return values, nil
}

func labelsFor(key engine.ResourceKey, runtimeName, installation string) map[string]string {
	labels := map[string]string{
		"orkest.managed": "agentctl", "orkest.runtime": runtimeName, "orkest.installation": installation,
	}
	for keyName, value := range map[string]string{"organization": key.Scope.OrganizationID, "workspace": key.Scope.WorkspaceID, "task": key.Scope.TaskID, "run": key.ExecutionID} {
		hash := sha256.Sum256([]byte(value))
		labels["orkest."+keyName+"_id_hash"] = hex.EncodeToString(hash[:])
	}
	labels[checkoutMountLayoutLabel] = checkoutMountLayout
	return labels
}

func containerName(runtimeName, runID string) string {
	hash := sha256.Sum256([]byte(runID))
	return "orkest-agentctl-" + runtimeName + "-" + hex.EncodeToString(hash[:])[:24]
}

func containerNamesForRun(runID string) []string {
	names := make([]string, 0, len(runtimecatalog.Providers())+1)
	hash := sha256.Sum256([]byte(runID))
	for _, provider := range runtimecatalog.Providers() {
		names = append(names, containerName(string(provider.Runtime()), runID))
	}
	names = append(names, "orkest-agentctl-"+hex.EncodeToString(hash[:])[:24])
	return names
}

func matchesScope(labels map[string]string, scope engine.Scope) bool {
	if labels["orkest.organization_id"] == scope.OrganizationID && labels["orkest.workspace_id"] == scope.WorkspaceID && labels["orkest.task_id"] == scope.TaskID {
		return true
	}
	for label, value := range map[string]string{"organization": scope.OrganizationID, "workspace": scope.WorkspaceID, "task": scope.TaskID} {
		hash := sha256.Sum256([]byte(value))
		if labels["orkest."+label+"_id_hash"] != hex.EncodeToString(hash[:]) {
			return false
		}
	}
	return true
}

func scopeMatches(a, b engine.Scope) bool {
	return (a.OrganizationID == b.OrganizationID || a.OrganizationID == dockerHashValue(b.OrganizationID)) &&
		(a.WorkspaceID == b.WorkspaceID || a.WorkspaceID == dockerHashValue(b.WorkspaceID)) &&
		(a.TaskID == b.TaskID || a.TaskID == dockerHashValue(b.TaskID))
}

func dockerHashValue(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func keyFromLabels(installation string, labels map[string]string) engine.ResourceKey {
	key := engine.ResourceKey{
		InstallationID: installation,
		Scope:          engine.Scope{OrganizationID: labels["orkest.organization_id"], WorkspaceID: labels["orkest.workspace_id"], TaskID: labels["orkest.task_id"]},
		Class:          engine.ResourceClassAgent, ExecutionID: labels["orkest.run_id"], Qualifier: labels["orkest.runtime"],
	}
	if key.Scope.OrganizationID == "" {
		key.Scope.OrganizationID = labels["orkest.organization_id_hash"]
		key.Scope.WorkspaceID = labels["orkest.workspace_id_hash"]
		key.Scope.TaskID = labels["orkest.task_id_hash"]
		key.Qualifier = "legacy-hashed-identity"
	}
	if key.ExecutionID == "" {
		key.ExecutionID = labels["orkest.run_id_hash"]
	}
	return key
}

func ensureCodexMountTarget(stateDir string) error {
	directory := filepath.Join(stateDir, "codex-auth")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create Codex auth mount directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("restrict Codex auth mount directory permissions: %w", err)
	}
	placeholder, err := os.OpenFile(filepath.Join(directory, "auth.json"), os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create Codex auth mount target: %w", err)
	}
	return placeholder.Close()
}

func publishedEndpoint(inspected container.InspectResponse, containerPort int) (string, error) {
	if inspected.NetworkSettings == nil {
		return "", errors.New("agentctl container has no network settings")
	}
	port := network.MustParsePort(fmt.Sprintf("%d/tcp", containerPort))
	for _, binding := range inspected.NetworkSettings.Ports[port] {
		if binding.HostIP.String() == "127.0.0.1" && binding.HostPort != "" {
			return "http://127.0.0.1:" + binding.HostPort, nil
		}
	}
	return "", errors.New("agentctl container has no loopback port binding")
}

func hashScope(value string) (string, error) {
	if value == "" {
		return "", errors.New("scope identifier is required")
	}
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:]), nil
}

func prepareProtectedCheckoutMounts(work string, writableRoots []string) ([]mount.Mount, error) {
	parent := filepath.Join(work, "repositories")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, fmt.Errorf("create checkout mount parent: %w", err)
	}
	if _, err := existingDirectory(work, parent); err != nil {
		return nil, fmt.Errorf("validate checkout mount parent: %w", err)
	}
	mounts := []mount.Mount{{Type: mount.TypeBind, Source: parent, Target: workMountPath + "/repositories", ReadOnly: true, BindOptions: &mount.BindOptions{ReadOnlyNonRecursive: true}}}
	for _, relative := range writableRoots {
		clean := filepath.Clean(relative)
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("%w: invalid writable repository root", engine.ErrInvalidSpec)
		}
		source, err := existingDirectory(work, filepath.Join(work, clean))
		if err != nil {
			return nil, fmt.Errorf("validate checkout mount: %w", err)
		}
		mounts = append(mounts, mount.Mount{Type: mount.TypeBind, Source: source, Target: workMountPath + "/" + filepath.ToSlash(clean)})
	}
	return mounts, nil
}

func existingDirectory(root, target string) (string, error) {
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", engine.ErrInvalidSpec
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", engine.ErrInvalidSpec
		}
	}
	return current, nil
}
