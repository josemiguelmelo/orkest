// Package local implements the native-process infrastructure engine.
package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	"github.com/josemiguelmelo/orkest/internal/platform/agentctllogs"
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
)

type Config struct {
	DataRoot string
}

type ExecutorEngine struct {
	dataRoot     string
	installation string
	mu           sync.Mutex
	processes    map[string]*os.Process
}

var _ engine.ExecutorEngine = (*ExecutorEngine)(nil)

func New(config Config) (*ExecutorEngine, error) {
	root := strings.TrimSpace(config.DataRoot)
	if root == "" {
		return nil, errors.New("local engine data root is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve local engine data root: %w", err)
	}
	hash := sha256.Sum256([]byte(root))
	return &ExecutorEngine{dataRoot: root, installation: hex.EncodeToString(hash[:]), processes: make(map[string]*os.Process)}, nil
}

func (e *ExecutorEngine) Capabilities() engine.Capabilities {
	return engine.Capabilities{
		Modes:                  []engine.ExecutionMode{engine.ExecutionModeNative},
		DurableWork:            true,
		DurableState:           true,
		PlatformWorkspaceSetup: true,
	}
}

func (e *ExecutorEngine) Validate(ctx context.Context, requirements engine.Requirements) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := engine.ValidateCapabilities(e.Capabilities(), requirements); err != nil {
		return err
	}
	info, err := os.Stat(e.dataRoot)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect local engine data root: %w", err)
	}
	if err == nil && !info.IsDir() {
		return errors.New("local engine data root is not a directory")
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
	work, state := taskstorage.Directories(e.dataRoot, request.Scope.OrganizationID, request.Scope.WorkspaceID, request.Scope.TaskID)
	workspace := engine.Workspace{
		Identity:             filepath.Base(filepath.Dir(work)),
		PreparationDirectory: work,
		WorkPath:             work,
		StatePath:            state,
		CompatibilityID:      filepath.Base(filepath.Dir(work)),
	}
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
				return engine.Workspace{}, fmt.Errorf("inspect local task workspace: %w", err)
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return engine.Workspace{}, fmt.Errorf("%w: task workspace path is not a regular directory", engine.ErrInvalidSpec)
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
			return engine.Workspace{}, fmt.Errorf("create durable local task directory: %w", err)
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return engine.Workspace{}, fmt.Errorf("restrict local task directory permissions: %w", err)
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
	if spec.Key.Class != engine.ResourceClassAgent || spec.Mode != engine.ExecutionModeNative {
		return resource, engine.Unsupported("Local supports native agent workloads only")
	}
	if spec.Key.InstallationID != "" && spec.Key.InstallationID != e.installation {
		return resource, engine.ErrIdentityMismatch
	}
	if spec.Key.Scope.TaskID == "" || spec.Key.ExecutionID == "" {
		return resource, fmt.Errorf("%w: task and run identity are required", engine.ErrInvalidSpec)
	}
	if !filepath.IsAbs(spec.Artifact.ExecutablePath) {
		return resource, fmt.Errorf("%w: native executable path must be absolute", engine.ErrInvalidSpec)
	}
	program, err := os.Stat(spec.Artifact.ExecutablePath)
	if err != nil {
		return resource, fmt.Errorf("inspect local engine executable: %w", err)
	}
	if program.IsDir() || program.Mode()&0o111 == 0 {
		return resource, fmt.Errorf("%w: native executable must be an executable file", engine.ErrInvalidSpec)
	}
	if spec.Restart != engine.RestartNever {
		return resource, engine.Unsupported("Local engine does not support workload restart policies")
	}
	if len(spec.Mounts) != 0 || len(spec.GeneratedFiles) != 0 || len(spec.CredentialBindings) != 0 {
		return resource, engine.Unsupported("Local engine does not provide isolated bind or credential mounts")
	}
	if err := e.Validate(ctx, spec.Requirements); err != nil {
		return resource, err
	}
	workspace, err := e.ResolveWorkspace(ctx, engine.WorkspaceRequest{
		Scope: spec.Key.Scope, Intent: engine.WorkspaceInspect,
		Required: engine.WorkspaceGuarantees{DurableWork: true, DurableState: true},
	})
	if err != nil {
		return resource, err
	}
	if !workspace.Exists || workspace.Identity != spec.Workspace.Identity || workspace.WorkPath != spec.Workspace.WorkPath || workspace.StatePath != spec.Workspace.StatePath {
		return resource, fmt.Errorf("%w: local task workspace changed before launch", engine.ErrIdentityMismatch)
	}
	arguments, endpoint, err := e.resolveArguments(spec)
	if err != nil {
		return resource, err
	}
	cmd := exec.Command(spec.Artifact.ExecutablePath, arguments...)
	cmd.Dir = workspace.WorkPath
	cmd.Stdin, cmd.Stdout = nil, io.Discard
	cmd.Env = e.environment(spec)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return resource, fmt.Errorf("capture local engine workload logs: %w", err)
	}
	if err = cmd.Start(); err != nil {
		return resource, fmt.Errorf("start local engine workload: %w", err)
	}
	resource.Ref.Key.InstallationID = e.installation
	resource.Ref.ConcreteID = strconv.Itoa(cmd.Process.Pid)
	resource.State = engine.ResourceRunning
	resource.Endpoint = endpoint
	logsDone := make(chan struct{})
	go func() {
		agentctllogs.Forward(stderr, "run_id", spec.Key.ExecutionID)
		close(logsDone)
	}()
	identity, identityErr := processIdentity(ctx, cmd.Process.Pid)
	if identityErr != nil {
		_ = cmd.Process.Kill()
		<-logsDone
		_ = cmd.Wait()
		return resource, fmt.Errorf("identify local engine workload: %w", identityErr)
	}
	resource.Ref.ConcreteID = strconv.Itoa(cmd.Process.Pid) + ":" + identity
	if err = e.writeRunIdentity(spec.Key.ExecutionID, resource.Ref.ConcreteID); err != nil {
		_ = cmd.Process.Kill()
		<-logsDone
		_ = cmd.Wait()
		return resource, fmt.Errorf("persist local engine workload identity: %w", err)
	}
	e.mu.Lock()
	e.processes[resource.Ref.ConcreteID] = cmd.Process
	e.mu.Unlock()
	go func() {
		<-logsDone
		_ = cmd.Wait()
		e.mu.Lock()
		delete(e.processes, resource.Ref.ConcreteID)
		e.mu.Unlock()
	}()
	return resource, nil
}

func (e *ExecutorEngine) resolveArguments(spec engine.LaunchSpec) ([]string, string, error) {
	endpoints := make(map[string]engine.Endpoint, len(spec.Endpoints))
	for _, endpoint := range spec.Endpoints {
		endpoints[endpoint.Name] = endpoint
	}
	resolved := make(map[string]string, len(endpoints))
	for name, endpoint := range endpoints {
		if endpoint.Protocol != "http" || endpoint.Ingress != engine.IngressWorkerControl || (endpoint.ListenHost != "" && endpoint.ListenHost != "127.0.0.1") {
			return nil, "", engine.Unsupported("Local endpoints must use loopback HTTP worker-control ingress")
		}
		port := endpoint.ListenPort
		if port == 0 {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				return nil, "", fmt.Errorf("reserve local engine endpoint: %w", err)
			}
			port = listener.Addr().(*net.TCPAddr).Port
			if err = listener.Close(); err != nil {
				return nil, "", fmt.Errorf("release local engine endpoint: %w", err)
			}
		}
		resolved[name] = fmt.Sprintf("127.0.0.1:%d", port)
	}
	arguments := make([]string, 0, len(spec.Arguments))
	for _, argument := range spec.Arguments {
		if argument.Kind == engine.ArgumentLiteral {
			arguments = append(arguments, argument.Value)
			continue
		}
		value, ok := resolved[argument.Endpoint]
		if !ok {
			return nil, "", fmt.Errorf("%w: endpoint %q was not resolved", engine.ErrInvalidSpec, argument.Endpoint)
		}
		arguments = append(arguments, value)
	}
	if len(resolved) == 0 {
		return arguments, "", nil
	}
	for _, endpoint := range spec.Endpoints {
		if endpoint.Name == "agentctl" {
			return arguments, "http://" + resolved[endpoint.Name], nil
		}
	}
	return arguments, "", nil
}

func (e *ExecutorEngine) environment(spec engine.LaunchSpec) []string {
	allowed := make(map[string]struct{}, len(spec.HostEnvironment.AllowedNames))
	if spec.HostEnvironment.InheritAllowlisted {
		for _, name := range spec.HostEnvironment.AllowedNames {
			allowed[name] = struct{}{}
		}
	}
	values := make([]string, 0, len(allowed)+len(spec.Environment))
	positions := make(map[string]int, len(allowed)+len(spec.Environment))
	if spec.HostEnvironment.InheritAllowlisted {
		for _, entry := range os.Environ() {
			name, _, ok := strings.Cut(entry, "=")
			if !ok {
				continue
			}
			if _, ok = allowed[name]; !ok {
				continue
			}
			if _, duplicate := positions[name]; duplicate {
				continue
			}
			positions[name] = len(values)
			values = append(values, entry)
		}
	}
	for name, value := range spec.Environment {
		entry := name + "=" + value
		if index, ok := positions[name]; ok {
			values[index] = entry
			continue
		}
		positions[name] = len(values)
		values = append(values, entry)
	}
	return values
}

func (e *ExecutorEngine) Inspect(ctx context.Context, lookup engine.Lookup) (engine.Observation, error) {
	key, err := e.lookupKey(lookup)
	if err != nil {
		return engine.Observation{}, err
	}
	identity := lookup.Ref.ConcreteID
	if identity == "" && key.ExecutionID != "" {
		identity, err = e.readRunIdentity(key.ExecutionID)
		if errors.Is(err, os.ErrNotExist) {
			return engine.Observation{State: engine.ObservationAbsent}, nil
		}
		if err != nil {
			return engine.Observation{}, err
		}
	}
	pid, fingerprint, err := parseIdentity(identity)
	if err != nil {
		return engine.Observation{}, err
	}
	actual, err := processIdentity(ctx, pid)
	if errors.Is(err, errProcessMissing) {
		return engine.Observation{State: engine.ObservationStopped, Resource: engine.Resource{Ref: engine.ResourceRef{Key: key, ConcreteID: identity}, State: engine.ResourceStopped}}, nil
	}
	if err != nil {
		return engine.Observation{}, err
	}
	if actual != fingerprint {
		return engine.Observation{}, fmt.Errorf("%w: local process ID was reused", engine.ErrIdentityMismatch)
	}
	return engine.Observation{State: engine.ObservationActive, Resource: engine.Resource{Ref: engine.ResourceRef{Key: key, ConcreteID: identity}, State: engine.ResourceRunning}}, nil
}

func (e *ExecutorEngine) List(ctx context.Context, selector engine.Selector) ([]engine.Resource, error) {
	if selector.InstallationID != "" && selector.InstallationID != e.installation {
		return nil, engine.ErrIdentityMismatch
	}
	if selector.Class == engine.ResourceClassEditor {
		return []engine.Resource{}, nil
	}
	if selector.Class != engine.ResourceClassAgent {
		return nil, fmt.Errorf("%w: Local engine list requires a resource class", engine.ErrInvalidSpec)
	}
	if selector.Scope != nil {
		return nil, engine.Unsupported("legacy Local run identity files do not persist task scope")
	}
	paths, err := filepath.Glob(filepath.Join(e.dataRoot, "runs", "*.pid"))
	if err != nil {
		return nil, fmt.Errorf("list local run identities: %w", err)
	}
	resources := make([]engine.Resource, 0, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		identityBytes, readErr := os.ReadFile(path)
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return nil, fmt.Errorf("read local run identity: %w", readErr)
		}
		identity := strings.TrimSpace(string(identityBytes))
		pid, expected, parseErr := parseIdentity(identity)
		if parseErr != nil {
			return nil, parseErr
		}
		actual, inspectErr := processIdentity(ctx, pid)
		if errors.Is(inspectErr, errProcessMissing) || (inspectErr == nil && actual != expected) {
			continue
		}
		if inspectErr != nil {
			return nil, inspectErr
		}
		hash := strings.TrimSuffix(filepath.Base(path), ".pid")
		key := engine.ResourceKey{InstallationID: e.installation, Class: engine.ResourceClassAgent, ExecutionID: hash, Qualifier: "run-id-sha256"}
		resources = append(resources, engine.Resource{Ref: engine.ResourceRef{Key: key, ConcreteID: identity}, State: engine.ResourceRunning})
	}
	return resources, nil
}

func (e *ExecutorEngine) Stop(ctx context.Context, ref engine.ResourceRef) error {
	if ref.Key.InstallationID != "" && ref.Key.InstallationID != e.installation {
		return engine.ErrIdentityMismatch
	}
	observation, err := e.Inspect(ctx, engine.Lookup{Ref: ref})
	if err != nil || observation.State == engine.ObservationAbsent || observation.State == engine.ObservationStopped {
		return err
	}
	identity := observation.Resource.Ref.ConcreteID
	pid, _, err := parseIdentity(identity)
	if err != nil {
		return err
	}
	e.mu.Lock()
	process := e.processes[identity]
	e.mu.Unlock()
	if process != nil {
		err = process.Signal(os.Interrupt)
	} else {
		err = signalProcess(pid)
	}
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("signal local engine workload: %w", err)
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		observation, err = e.Inspect(ctx, engine.Lookup{Ref: ref})
		if err != nil {
			return err
		}
		if observation.State != engine.ObservationActive {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (*ExecutorEngine) Cleanup(context.Context, engine.ResourceRef) error { return nil }
func (*ExecutorEngine) Close() error                                      { return nil }

func (e *ExecutorEngine) lookupKey(lookup engine.Lookup) (engine.ResourceKey, error) {
	key := lookup.Ref.Key
	if lookup.Key != nil {
		key = *lookup.Key
	}
	if key.InstallationID != "" && key.InstallationID != e.installation {
		return engine.ResourceKey{}, engine.ErrIdentityMismatch
	}
	if key.Class != engine.ResourceClassUnknown && key.Class != engine.ResourceClassAgent {
		return engine.ResourceKey{}, engine.ErrIdentityMismatch
	}
	key.InstallationID = e.installation
	return key, nil
}

func (e *ExecutorEngine) writeRunIdentity(runID, identity string) error {
	path := e.identityPath(runID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create local run identity directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(identity), 0o600); err != nil {
		return fmt.Errorf("write local run identity: %w", err)
	}
	return nil
}

func (e *ExecutorEngine) readRunIdentity(runID string) (string, error) {
	data, err := os.ReadFile(e.identityPath(runID))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (e *ExecutorEngine) identityPath(runID string) string {
	hash := sha256.Sum256([]byte(runID))
	return filepath.Join(e.dataRoot, "runs", hex.EncodeToString(hash[:])+".pid")
}

func parseIdentity(value string) (int, string, error) {
	pidValue, token, ok := strings.Cut(strings.TrimSpace(value), ":")
	if !ok || token == "" {
		return 0, "", errors.New("invalid local engine process identity")
	}
	pid, err := strconv.Atoi(pidValue)
	if err != nil || pid < 1 {
		return 0, "", errors.New("invalid local engine process identity")
	}
	return pid, token, nil
}
