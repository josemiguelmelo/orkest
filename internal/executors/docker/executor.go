// Package docker implements the first task-agent process executor using Docker Engine.
package docker

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/josemiguelmelo/orkest/internal/agentctl"
	"github.com/josemiguelmelo/orkest/internal/orchestration"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	mobyclient "github.com/moby/moby/client"
)

const (
	DefaultAgentctlPort = 8081
	workMountPath       = "/workspace"
	stateMountPath      = "/agentctl-state"
	defaultImage        = "orkest/agentctl-opencode:2.1.4"
)

type EngineClient interface {
	ContainerCreate(context.Context, mobyclient.ContainerCreateOptions) (mobyclient.ContainerCreateResult, error)
	ContainerStart(context.Context, string, mobyclient.ContainerStartOptions) (mobyclient.ContainerStartResult, error)
	ContainerInspect(context.Context, string, mobyclient.ContainerInspectOptions) (mobyclient.ContainerInspectResult, error)
	ContainerRemove(context.Context, string, mobyclient.ContainerRemoveOptions) (mobyclient.ContainerRemoveResult, error)
	Close() error
}

type Config struct {
	DataRoot      string
	Image         string
	AgentctlPort  int
	ContainerUser string
}

type Executor struct {
	client EngineClient
	config Config
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
	image := strings.TrimSpace(config.Image)
	if image == "" {
		image = defaultImage
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
	config.DataRoot, config.Image, config.AgentctlPort, config.ContainerUser = root, image, port, containerUser
	return &Executor{client: client, config: config}, nil
}

var _ orchestration.ProcessExecutor = (*Executor)(nil)

func (e *Executor) StartAgentctl(ctx context.Context, config agentctl.RunConfig) (orchestration.Process, error) {
	if err := config.Validate(); err != nil {
		return orchestration.Process{}, err
	}
	workDir, stateDir, err := e.taskDirectories(config)
	if err != nil {
		return orchestration.Process{}, err
	}
	labels, err := labelsFor(config)
	if err != nil {
		return orchestration.Process{}, err
	}
	port := network.MustParsePort(fmt.Sprintf("%d/tcp", e.config.AgentctlPort))
	containerConfig := &container.Config{
		Image: e.config.Image,
		User:  e.config.ContainerUser,
		Cmd: []string{
			"-listen", fmt.Sprintf("0.0.0.0:%d", e.config.AgentctlPort),
			"-work-dir", workMountPath,
			"-state-dir", stateMountPath,
		},
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
	name := containerName(config.RunID)
	created, err := e.client.ContainerCreate(ctx, mobyclient.ContainerCreateOptions{
		Config:     containerConfig,
		HostConfig: hostConfig,
		Name:       name,
	})
	if err != nil {
		// The deterministic name is a usable reconciliation identity when the
		// daemon created the container but the API response was lost.
		return orchestration.Process{ID: name}, fmt.Errorf("create agentctl container: %w", err)
	}
	process := orchestration.Process{ID: created.ID}
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
	if err != nil {
		return process, e.removeAfterFailure(ctx, created.ID, err)
	}
	process.Endpoint = endpoint
	return process, nil
}

func (e *Executor) Close() error { return e.client.Close() }

func (e *Executor) taskDirectories(config agentctl.RunConfig) (string, string, error) {
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

func labelsFor(config agentctl.RunConfig) (map[string]string, error) {
	values := []string{config.OrganizationID, config.WorkspaceID, config.TaskID, config.RunID}
	labels := make(map[string]string, len(values)+1)
	for i, key := range []string{"organization", "workspace", "task", "run"} {
		hash, err := hashScope(values[i])
		if err != nil {
			return nil, err
		}
		labels["orkest."+key+"_id_hash"] = hash
	}
	labels["orkest.managed"] = "agentctl"
	return labels, nil
}

func containerName(runID string) string {
	hash, _ := hashScope(runID)
	return "orkest-agentctl-" + hash[:24]
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
