// Package docker implements the first task-agent process executor using Docker Engine.
package docker

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/josemiguelmelo/orkest/internal/adapters/executors/checkout"
	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/platform/agentctllogs"
	"github.com/josemiguelmelo/orkest/internal/server/coderepository"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	mountmodel "github.com/josemiguelmelo/orkest/internal/server/workspacemount/model"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	mobyclient "github.com/moby/moby/client"
)

const (
	DefaultAgentctlPort      = 8081
	workMountPath            = "/workspace"
	stateMountPath           = "/agentctl-state"
	codexAuthMountPath       = stateMountPath + "/codex-auth/auth.json"
	codexAuthFileEnvironment = "ORKEST_CODEX_AUTH_FILE"
)

type EngineClient interface {
	ContainerCreate(context.Context, mobyclient.ContainerCreateOptions) (mobyclient.ContainerCreateResult, error)
	ContainerStart(context.Context, string, mobyclient.ContainerStartOptions) (mobyclient.ContainerStartResult, error)
	ContainerStop(context.Context, string, mobyclient.ContainerStopOptions) (mobyclient.ContainerStopResult, error)
	ContainerInspect(context.Context, string, mobyclient.ContainerInspectOptions) (mobyclient.ContainerInspectResult, error)
	ContainerLogs(context.Context, string, mobyclient.ContainerLogsOptions) (mobyclient.ContainerLogsResult, error)
	ContainerRemove(context.Context, string, mobyclient.ContainerRemoveOptions) (mobyclient.ContainerRemoveResult, error)
	Close() error
}

type Config struct {
	DataRoot                 string
	NetworkName              string
	Images                   map[coreruntime.Runtime]string
	AgentctlPort             int
	ContainerUser            string
	CodexAuthFile            string
	ProviderEnv              []string
	CodingProviders          []coderepository.CodingProvider
	CodingCredentialResolver checkout.CredentialResolver
}

type Executor struct {
	client              EngineClient
	config              Config
	repositoryProviders map[string]checkout.Provider
}

func New(ctx context.Context, config Config) (*Executor, error) {
	client, err := mobyclient.New(mobyclient.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("configure Docker Engine client: %w", err)
	}
	executor, err := NewWithClient(ctx, config, client)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return executor, nil
}

func NewWithClient(ctx context.Context, config Config, client EngineClient) (*Executor, error) {
	if ctx == nil || client == nil {
		return nil, errors.New("context and Docker Engine client are required")
	}
	root := strings.TrimSpace(config.DataRoot)
	if root == "" {
		return nil, errors.New("data root is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve Docker data root: %w", err)
	}
	images := make(map[coreruntime.Runtime]string, len(coreruntime.Providers()))
	for _, provider := range coreruntime.Providers() {
		images[provider.Runtime()] = provider.DefaultImage()
	}
	for runtime, image := range config.Images {
		if !runtime.Available() {
			return nil, fmt.Errorf("unsupported runtime image %q", runtime)
		}
		if image = strings.TrimSpace(image); image != "" {
			images[runtime] = image
		}
	}
	port := config.AgentctlPort
	if port == 0 {
		port = DefaultAgentctlPort
	}
	if port < 1 || port > 65535 {
		return nil, errors.New("agentctl container port must be between 1 and 65535")
	}
	containerUser := strings.TrimSpace(config.ContainerUser)
	if containerUser == "" {
		containerUser, err = currentContainerUser()
		if err != nil {
			return nil, err
		}
	}
	if err := validateContainerUser(containerUser); err != nil {
		return nil, err
	}
	codexAuthFile := strings.TrimSpace(config.CodexAuthFile)
	if codexAuthFile != "" && !filepath.IsAbs(codexAuthFile) {
		return nil, errors.New("codex auth file path must be absolute")
	}
	config.DataRoot, config.Images, config.AgentctlPort, config.ContainerUser, config.CodexAuthFile = root, images, port, containerUser, codexAuthFile
	config.NetworkName = strings.TrimSpace(config.NetworkName)
	providers := make(map[string]checkout.Provider, len(config.CodingProviders))
	for _, provider := range config.CodingProviders {
		if provider == nil || strings.TrimSpace(provider.ProviderName()) == "" {
			return nil, errors.New("code repository provider name is required")
		}
		providers[provider.ProviderName()] = provider
	}
	return &Executor{client: client, config: config, repositoryProviders: providers}, nil
}

var (
	_ orchestration.AgentctlStarter        = (*Executor)(nil)
	_ orchestration.AgentProcessController = (*Executor)(nil)
)

func (e *Executor) StartAgentctl(ctx context.Context, config orchestrationmodel.RunConfig) (orchestrationmodel.Process, error) {
	if err := config.Validate(); err != nil {
		return orchestrationmodel.Process{}, err
	}
	if err := mountmodel.Validate(config.HostMounts); err != nil {
		return orchestrationmodel.Process{}, fmt.Errorf("validate Docker host mounts: %w", err)
	}
	image := config.ExecutionImage
	if image == "" {
		image = e.config.Images[config.Runtime]
	}
	if image == "" {
		return orchestrationmodel.Process{}, fmt.Errorf("no Docker image configured for runtime %q", config.Runtime)
	}
	slog.Default().Info("starting agentctl container", "organization_id", config.OrganizationID, "workspace_id", config.WorkspaceID, "task_id", config.TaskID, "run_id", config.RunID, "runtime", config.Runtime, "image", image)
	workDir, stateDir, err := e.taskDirectories(config)
	if err != nil {
		return orchestrationmodel.Process{}, err
	}
	if err = e.prepareRepositories(ctx, workDir, config); err != nil {
		return orchestrationmodel.Process{}, fmt.Errorf("prepare task repositories: %w", err)
	}
	labels, err := labelsFor(config)
	if err != nil {
		return orchestrationmodel.Process{}, err
	}
	port := network.MustParsePort(fmt.Sprintf("%d/tcp", e.config.AgentctlPort))
	providerEnv := e.providerEnvironment()
	containerEnv := append([]string(nil), providerEnv...)
	command := []string{
		"-listen", fmt.Sprintf("0.0.0.0:%d", e.config.AgentctlPort),
		"-runtime", string(config.Runtime),
		"-work-dir", workMountPath,
		"-state-dir", stateMountPath,
	}
	if config.TaskSessionID != "" {
		command = append(command, "-run-id", config.RunID)
	}
	if config.Runtime == coreruntime.Codex && e.config.CodexAuthFile != "" {
		containerEnv = append(containerEnv, codexAuthFileEnvironment+"="+codexAuthMountPath)
		command = append(command, "-env", codexAuthFileEnvironment)
	}
	for _, variable := range providerEnv {
		name, _, _ := strings.Cut(variable, "=")
		command = append(command, "-env", name)
	}
	containerConfig := &container.Config{
		Image:        image,
		User:         e.config.ContainerUser,
		Env:          containerEnv,
		Cmd:          command,
		ExposedPorts: network.PortSet{port: {}},
		Labels:       labels,
	}
	hostConfig := &container.HostConfig{
		Mounts: []mount.Mount{
			{Type: mount.TypeBind, Source: workDir, Target: workMountPath},
			{Type: mount.TypeBind, Source: stateDir, Target: stateMountPath},
		},
		PortBindings: network.PortMap{port: {{HostIP: netip.MustParseAddr("127.0.0.1")}}},
		// Agentctl cannot restore its active session after process restart yet. Do not
		// silently restart an incomplete run until reconciliation is implemented.
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyDisabled},
	}
	hostMounts, err := bindMounts(config, stateDir)
	if err != nil {
		return orchestrationmodel.Process{}, err
	}
	hostConfig.Mounts = append(hostConfig.Mounts, hostMounts...)
	if config.Runtime == coreruntime.Codex && e.config.CodexAuthFile != "" {
		hostConfig.Mounts = append(hostConfig.Mounts, mount.Mount{
			Type: mount.TypeBind, Source: e.config.CodexAuthFile, Target: codexAuthMountPath, ReadOnly: true,
		})
	}
	if e.config.NetworkName != "" {
		hostConfig.NetworkMode = container.NetworkMode(e.config.NetworkName)
		hostConfig.PortBindings = nil
	}
	if config.TaskSessionID != "" {
		hostConfig.RestartPolicy.Name = container.RestartPolicyUnlessStopped
	}
	name := containerName(config.Runtime, config.RunID)
	created, err := e.client.ContainerCreate(ctx, mobyclient.ContainerCreateOptions{
		Config:     containerConfig,
		HostConfig: hostConfig,
		Name:       name,
	})
	if err != nil {
		// The deterministic name is a usable reconciliation identity when the
		// daemon created the container but the API response was lost.
		return orchestrationmodel.Process{ID: name}, fmt.Errorf("create agentctl container: %w", err)
	}
	process := orchestrationmodel.Process{ID: created.ID}
	if err = ctx.Err(); err != nil {
		return process, e.removeAfterFailure(ctx, created.ID, fmt.Errorf("start agentctl container: %w", err))
	}
	_, startErr := e.client.ContainerStart(ctx, created.ID, mobyclient.ContainerStartOptions{})
	inspection, inspectErr := e.client.ContainerInspect(ctx, created.ID, mobyclient.ContainerInspectOptions{})
	if inspectErr != nil {
		if startErr != nil {
			return process, errors.Join(fmt.Errorf("start agentctl container: %w", startErr), fmt.Errorf("inspect uncertain container state: %w", inspectErr))
		}
		// A successful start followed by an inspect failure does not prove that the
		// container stopped. Leave it for reconciliation and retain its identity.
		return process, fmt.Errorf("inspect started agentctl container: %w", inspectErr)
	}
	if inspection.Container.State == nil {
		failure := errors.New("docker did not report agentctl container state")
		if startErr != nil {
			failure = errors.Join(fmt.Errorf("start agentctl container: %w", startErr), failure)
		}
		return process, failure
	}
	if !inspection.Container.State.Running {
		failure := startErr
		if failure == nil {
			failure = errors.New("agentctl container exited before becoming reachable")
		}
		return process, e.removeAfterFailure(ctx, created.ID, fmt.Errorf("start agentctl container: %w", failure))
	}
	endpoint, err := publishedEndpoint(inspection.Container, e.config.AgentctlPort)
	if e.config.NetworkName != "" {
		endpoint = fmt.Sprintf("http://%s:%d", name, e.config.AgentctlPort)
		err = nil
	}
	if err != nil {
		return process, e.removeAfterFailure(ctx, created.ID, err)
	}
	process.Endpoint = endpoint
	go e.forwardAgentctlLogs(created.ID)
	return process, nil
}

func (e *Executor) prepareRepositories(ctx context.Context, workDir string, config orchestrationmodel.RunConfig) error {
	preparer := checkout.Preparer{Providers: e.repositoryProviders, Credentials: e.config.CodingCredentialResolver}
	return preparer.Prepare(ctx, workDir, config.WorkspaceID, config.Repositories)
}

func (e *Executor) providerEnvironment() []string {
	values := make([]string, 0, len(e.config.ProviderEnv))
	seen := make(map[string]struct{}, len(e.config.ProviderEnv))
	for _, name := range e.config.ProviderEnv {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if name == codexAuthFileEnvironment {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		if value, ok := os.LookupEnv(name); ok {
			values = append(values, name+"="+value)
		}
	}
	return values
}

func (e *Executor) Close() error { return e.client.Close() }

// TerminateAgentctl gracefully stops agentctl so it can close its runtime
// subprocess, then removes the stopped container while retaining bind-mounted data.
func (e *Executor) TerminateAgentctl(ctx context.Context, processID string) error {
	if strings.TrimSpace(processID) == "" {
		return errors.New("agentctl process identity is required")
	}
	inspection, err := e.client.ContainerInspect(ctx, processID, mobyclient.ContainerInspectOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("inspect agentctl container before shutdown: %w", err)
	}
	if inspection.Container.State == nil {
		return errors.New("docker did not report agentctl container state during shutdown")
	}
	if inspection.Container.State.Running {
		timeout := 10
		if _, err = e.client.ContainerStop(ctx, processID, mobyclient.ContainerStopOptions{Timeout: &timeout}); err != nil {
			if errdefs.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("stop agentctl container: %w", err)
		}
	}
	if _, err = e.client.ContainerRemove(ctx, processID, mobyclient.ContainerRemoveOptions{}); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("remove stopped agentctl container: %w", err)
	}
	return nil
}

// forwardAgentctlLogs follows the container stderr stream, where agentctl emits
// structured logs. The shared filter drops provider output and sensitive fields.
func (e *Executor) forwardAgentctlLogs(processID string) {
	logs, err := e.client.ContainerLogs(context.Background(), processID, mobyclient.ContainerLogsOptions{ShowStderr: true, Follow: true})
	if err != nil {
		slog.Default().Warn("follow agentctl container logs failed", "container_id", processID, "error", err)
		return
	}
	defer func() { _ = logs.Close() }()
	reader, writer := io.Pipe()
	go func() {
		_, copyErr := stdcopy.StdCopy(io.Discard, writer, logs)
		_ = writer.CloseWithError(copyErr)
	}()
	agentctllogs.Forward(reader, "container_id", processID)
}

// TerminateAgentctlByRunID resolves the deterministic container name used before
// the Docker ID is persisted, covering a platform crash during container startup.
func (e *Executor) TerminateAgentctlByRunID(ctx context.Context, runID string) error {
	if strings.TrimSpace(runID) == "" {
		return errors.New("agent run identity is required")
	}
	var failures []error
	for _, name := range containerNamesForRun(runID) {
		if err := e.TerminateAgentctl(ctx, name); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (e *Executor) AgentctlRunning(ctx context.Context, processID string) (bool, error) {
	if strings.TrimSpace(processID) == "" {
		return false, errors.New("agentctl process identity is required")
	}
	inspection, err := e.client.ContainerInspect(ctx, processID, mobyclient.ContainerInspectOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect agentctl container state: %w", err)
	}
	if inspection.Container.State == nil {
		return false, errors.New("docker did not report agentctl container state")
	}
	return inspection.Container.State.Running, nil
}

func (e *Executor) AgentctlRunningByRunID(ctx context.Context, runID string) (bool, error) {
	if strings.TrimSpace(runID) == "" {
		return false, errors.New("agent run identity is required")
	}
	for _, name := range containerNamesForRun(runID) {
		running, err := e.AgentctlRunning(ctx, name)
		if err != nil || running {
			return running, err
		}
	}
	return false, nil
}

func (e *Executor) taskDirectories(config orchestrationmodel.RunConfig) (string, string, error) {
	scope, err := json.Marshal([]string{config.OrganizationID, config.WorkspaceID, config.TaskID})
	if err != nil {
		return "", "", fmt.Errorf("encode task storage scope: %w", err)
	}
	taskDir := filepath.Join(e.config.DataRoot, "tasks", fmt.Sprintf("%x", sha256.Sum256(scope)))
	workDir, stateDir := filepath.Join(taskDir, "work"), filepath.Join(taskDir, "agentctl-state")
	for _, path := range []string{workDir, stateDir} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return "", "", fmt.Errorf("create durable task directory: %w", err)
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return "", "", fmt.Errorf("restrict durable task directory permissions: %w", err)
		}
	}
	if config.Runtime == coreruntime.Codex && e.config.CodexAuthFile != "" {
		authMountDir := filepath.Join(stateDir, "codex-auth")
		if err := os.MkdirAll(authMountDir, 0o700); err != nil {
			return "", "", fmt.Errorf("create Codex auth mount directory: %w", err)
		}
		if err := os.Chmod(authMountDir, 0o700); err != nil {
			return "", "", fmt.Errorf("restrict Codex auth mount directory permissions: %w", err)
		}
		placeholder, err := os.OpenFile(filepath.Join(authMountDir, "auth.json"), os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return "", "", fmt.Errorf("create Codex auth mount target: %w", err)
		}
		if err := placeholder.Close(); err != nil {
			return "", "", fmt.Errorf("close Codex auth mount target: %w", err)
		}
	}
	return workDir, stateDir, nil
}

func (e *Executor) removeAfterFailure(ctx context.Context, containerID string, cause error) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if _, cleanupErr := e.client.ContainerRemove(cleanupCtx, containerID, mobyclient.ContainerRemoveOptions{Force: true}); cleanupErr != nil {
		return errors.Join(cause, fmt.Errorf("remove incomplete agentctl container: %w", cleanupErr))
	}
	return cause
}

func labelsFor(config orchestrationmodel.RunConfig) (map[string]string, error) {
	values := []string{config.OrganizationID, config.WorkspaceID, config.TaskID, config.RunID}
	labels := make(map[string]string, len(values)+2)
	for i, key := range []string{"organization", "workspace", "task", "run"} {
		hash, err := hashScope(values[i])
		if err != nil {
			return nil, err
		}
		labels["orkest."+key+"_id_hash"] = hash
	}
	labels["orkest.managed"] = "agentctl"
	labels["orkest.runtime"] = string(config.Runtime)
	return labels, nil
}

func containerName(runtime coreruntime.Runtime, runID string) string {
	hash, _ := hashScope(runID)
	return "orkest-agentctl-" + string(runtime) + "-" + hash[:24]
}

func containerNamesForRun(runID string) []string {
	names := make([]string, 0, len(coreruntime.Providers())+1)
	for _, provider := range coreruntime.Providers() {
		names = append(names, containerName(provider.Runtime(), runID))
	}
	// Check the pre-runtime-qualified name as well so an upgrade can reconcile
	// containers created before runtime names were introduced.
	hash, _ := hashScope(runID)
	names = append(names, "orkest-agentctl-"+hash[:24])
	return names
}

func hashScope(value string) (string, error) {
	if value == "" {
		return "", errors.New("scope identifier is required")
	}
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum), nil
}

func publishedEndpoint(inspected container.InspectResponse, containerPort int) (string, error) {
	if inspected.NetworkSettings == nil {
		return "", errors.New("agentctl container has no network settings")
	}
	port := network.MustParsePort(fmt.Sprintf("%d/tcp", containerPort))
	bindings := inspected.NetworkSettings.Ports[port]
	for _, binding := range bindings {
		if binding.HostIP.String() == "127.0.0.1" && binding.HostPort != "" {
			return "http://127.0.0.1:" + binding.HostPort, nil
		}
	}
	return "", errors.New("agentctl container has no loopback port binding")
}

func currentContainerUser() (string, error) {
	current, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("resolve current user for container file ownership: %w", err)
	}
	if _, err = strconv.Atoi(current.Uid); err != nil {
		return "", errors.New("docker execution requires a numeric local user ID")
	}
	if _, err = strconv.Atoi(current.Gid); err != nil {
		return "", errors.New("docker execution requires a numeric local group ID")
	}
	return current.Uid + ":" + current.Gid, nil
}

func validateContainerUser(value string) error {
	uid, gid, ok := strings.Cut(value, ":")
	if !ok {
		return errors.New("container user must use numeric uid:gid format")
	}
	if _, err := strconv.ParseUint(uid, 10, 32); err != nil {
		return errors.New("container user must use numeric uid:gid format")
	}
	if _, err := strconv.ParseUint(gid, 10, 32); err != nil {
		return errors.New("container user must use numeric uid:gid format")
	}
	return nil
}
