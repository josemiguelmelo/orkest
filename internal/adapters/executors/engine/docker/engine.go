// Package docker implements container infrastructure for shared executors.
package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	client "github.com/moby/moby/client"
)

type ContainerClient interface {
	ContainerCreate(context.Context, client.ContainerCreateOptions) (client.ContainerCreateResult, error)
	ContainerStart(context.Context, string, client.ContainerStartOptions) (client.ContainerStartResult, error)
	ContainerStop(context.Context, string, client.ContainerStopOptions) (client.ContainerStopResult, error)
	ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerRemove(context.Context, string, client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
	ContainerLogs(context.Context, string, client.ContainerLogsOptions) (client.ContainerLogsResult, error)
	ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error)
	Close() error
}

type NetworkClient interface {
	NetworkCreate(context.Context, string, client.NetworkCreateOptions) (client.NetworkCreateResult, error)
	NetworkInspect(context.Context, string, client.NetworkInspectOptions) (client.NetworkInspectResult, error)
	NetworkConnect(context.Context, string, client.NetworkConnectOptions) (client.NetworkConnectResult, error)
	NetworkDisconnect(context.Context, string, client.NetworkDisconnectOptions) (client.NetworkDisconnectResult, error)
	NetworkRemove(context.Context, string, client.NetworkRemoveOptions) (client.NetworkRemoveResult, error)
	NetworkList(context.Context, client.NetworkListOptions) (client.NetworkListResult, error)
}

type Client interface {
	ContainerClient
	NetworkClient
}

type Config struct {
	DataRoot          string
	NetworkName       string
	ContainerUser     string
	CredentialSources map[string]engine.CredentialSource
	LogForwarder      engine.LogForwarder
	Client            Client
}

type Engine struct {
	client        Client
	networkClient Client
	manifests     *engine.ManifestStore
	config        Config
	installation  string
	closeOnce     sync.Once
	closeErr      error
}

var _ engine.InfrastructureEngine = (*Engine)(nil)

// NewClient constructs the Docker API client owned by the infrastructure engine.
func NewClient() (Client, error) { return client.New(client.FromEnv) }

func New(config Config) (*Engine, error) {
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
	for name, source := range config.CredentialSources {
		if strings.TrimSpace(name) == "" || source.FilePath == "" || !filepath.IsAbs(source.FilePath) {
			return nil, errors.New("credential sources require a name and absolute file path")
		}
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
	manifests, err := engine.NewManifestStore(filepath.Join(config.DataRoot, "infrastructure-operations"))
	if err != nil {
		return nil, err
	}
	return &Engine{client: config.Client, networkClient: config.Client, config: config, installation: hex.EncodeToString(hash[:]), manifests: manifests}, nil
}

func (e *Engine) Capabilities() engine.Capabilities {
	return engine.Capabilities{
		Modes:       []engine.ExecutionMode{engine.ExecutionModeContainer},
		DurableWork: true, DurableState: true, PlatformStorageSetup: true, ReadOnlyRoot: true, ReadOnlyMounts: true,
		TemporaryMounts: true, ProtectedStorageSharing: true,
		ConfirmedProcessTreeStop: true,
		ExplicitHostBinds:        true, CredentialBindings: len(e.config.CredentialSources) > 0, RestartModes: true,
		PlatformIngress:       e.networkClient != nil,
		DeniedEgress:          e.networkClient != nil,
		PrivateServiceRouting: e.networkClient != nil,
	}
}

func (e *Engine) Close() error {
	e.closeOnce.Do(func() { e.closeErr = e.client.Close() })
	return e.closeErr
}

func (e *Engine) containerUser(requirements engine.SecurityRequirements) (string, error) {
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
		return "", engine.Unsupported("Docker workloads require a non-root identity")
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

func ensureCredentialMountTarget(stateDir, target string) error {
	clean := filepath.Clean(target)
	if !filepath.IsAbs(clean) || clean == string(filepath.Separator) {
		return errors.New("credential target must be an absolute file path")
	}
	relative := strings.TrimPrefix(clean, string(filepath.Separator))
	directory := filepath.Join(stateDir, filepath.Dir(relative))
	if !pathWithin(stateDir, directory) {
		return errors.New("credential target escapes the workload mount root")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create credential mount directory: %w", err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("restrict credential mount directory permissions: %w", err)
	}
	placeholder, err := os.OpenFile(filepath.Join(stateDir, relative), os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create credential mount target: %w", err)
	}
	return placeholder.Close()
}

func publishedEndpoint(inspected container.InspectResponse, containerPort int) (string, error) {
	if inspected.NetworkSettings == nil {
		return "", errors.New("workload has no network settings")
	}
	port := network.MustParsePort(fmt.Sprintf("%d/tcp", containerPort))
	for _, binding := range inspected.NetworkSettings.Ports[port] {
		if binding.HostIP.String() == "127.0.0.1" && binding.HostPort != "" {
			return "http://127.0.0.1:" + binding.HostPort, nil
		}
	}
	return "", errors.New("workload has no loopback port binding")
}
