package docker

import (
	"context"

	client "github.com/moby/moby/client"
)

// ContainerClient contains operations shared by agent and editor containers.
// Client ownership and worker log streaming are deliberately separate.
type ContainerClient interface {
	ContainerCreate(context.Context, client.ContainerCreateOptions) (client.ContainerCreateResult, error)
	ContainerStart(context.Context, string, client.ContainerStartOptions) (client.ContainerStartResult, error)
	ContainerStop(context.Context, string, client.ContainerStopOptions) (client.ContainerStopResult, error)
	ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error)
	ContainerRemove(context.Context, string, client.ContainerRemoveOptions) (client.ContainerRemoveResult, error)
}

// EngineClient is the Docker surface used by the agent executor.
type EngineClient interface {
	ContainerClient
	ContainerLogs(context.Context, string, client.ContainerLogsOptions) (client.ContainerLogsResult, error)
	Close() error
}

// EditorEngineClient is borrowed by the editor backend; it cannot close the client.
type EditorEngineClient interface {
	ContainerClient
	ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error)
	NetworkCreate(context.Context, string, client.NetworkCreateOptions) (client.NetworkCreateResult, error)
	NetworkInspect(context.Context, string, client.NetworkInspectOptions) (client.NetworkInspectResult, error)
	NetworkConnect(context.Context, string, client.NetworkConnectOptions) (client.NetworkConnectResult, error)
	NetworkDisconnect(context.Context, string, client.NetworkDisconnectOptions) (client.NetworkDisconnectResult, error)
	NetworkRemove(context.Context, string, client.NetworkRemoveOptions) (client.NetworkRemoveResult, error)
	NetworkList(context.Context, client.NetworkListOptions) (client.NetworkListResult, error)
}

// Engine is the complete infrastructure client shared at composition.
type Engine interface {
	EngineClient
	EditorEngineClient
}

// NewEngineClient constructs an owned client without starting Docker resources.
func NewEngineClient() (Engine, error) {
	return client.New(client.FromEnv)
}
