package docker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/containerd/errdefs"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	client "github.com/moby/moby/client"
)

type cleanupEngine struct {
	editorEngine
	network               network.Network
	current               container.InspectResponse
	inspectErr            error
	endpoints             map[string]network.EndpointResource
	disconnected, removed bool
}

func (*cleanupEngine) ContainerList(context.Context, client.ContainerListOptions) (client.ContainerListResult, error) {
	// This snapshot predates an editor opened in another workspace.
	return client.ContainerListResult{}, nil
}

func (e *cleanupEngine) ContainerInspect(context.Context, string, client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	return client.ContainerInspectResult{Container: e.current}, e.inspectErr
}

func (e *cleanupEngine) NetworkList(context.Context, client.NetworkListOptions) (client.NetworkListResult, error) {
	return client.NetworkListResult{Items: []network.Summary{{Network: e.network}}}, nil
}

func (e *cleanupEngine) NetworkInspect(context.Context, string, client.NetworkInspectOptions) (client.NetworkInspectResult, error) {
	return client.NetworkInspectResult{Network: network.Inspect{Network: e.network, Containers: e.endpoints}}, nil
}

func (e *cleanupEngine) NetworkDisconnect(context.Context, string, client.NetworkDisconnectOptions) (client.NetworkDisconnectResult, error) {
	e.disconnected = true
	return client.NetworkDisconnectResult{}, nil
}

func (e *cleanupEngine) NetworkRemove(context.Context, string, client.NetworkRemoveOptions) (client.NetworkRemoveResult, error) {
	e.removed = true
	return client.NetworkRemoveResult{}, nil
}

func cleanupFixture() (*EditorBackend, *cleanupEngine, taskeditor.Instance) {
	engine := &cleanupEngine{endpoints: map[string]network.EndpointResource{"gateway": {}}}
	backend := &EditorBackend{engine: engine, installation: "installation"}
	i := taskeditor.Instance{Scope: taskeditor.Scope{OrganizationID: "org", WorkspaceID: "other-workspace", TaskID: "task"}, UserID: "user", Generation: strings.Repeat("a", 64), Mode: taskeditor.ReadWrite}
	labels := backend.labels(i)
	labels[editorLabel+".gateway"] = "gateway"
	engine.network = network.Network{ID: "network", Name: backend.networkName(i), Driver: "bridge", Internal: true, Labels: labels}
	engine.current = container.InspectResponse{ID: "editor", Config: &container.Config{Labels: labels}}
	return backend, engine, i
}

func TestEditorCleanupRechecksStaleContainerSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name             string
		inspectErr       error
		cleanup, wantErr bool
	}{
		{name: "new live editor"},
		{name: "uncertain ownership", inspectErr: errors.New("daemon unavailable"), wantErr: true},
		{name: "confirmed orphan", inspectErr: errdefs.ErrNotFound, cleanup: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend, engine, _ := cleanupFixture()
			engine.inspectErr = tc.inspectErr
			_, err := backend.ListEditors(context.Background())
			if (err != nil) != tc.wantErr || engine.disconnected != tc.cleanup || engine.removed != tc.cleanup {
				t.Fatalf("err=%v disconnected=%v removed=%v", err, engine.disconnected, engine.removed)
			}
		})
	}
}

func TestEditorNetworkRemovalPreservesGatewayWithLiveEndpoints(t *testing.T) {
	backend, engine, i := cleanupFixture()
	engine.endpoints["live-editor"] = network.EndpointResource{}
	if err := backend.removeNetwork(context.Background(), i); err == nil || engine.disconnected || engine.removed {
		t.Fatalf("live network changed: err=%v disconnected=%v removed=%v", err, engine.disconnected, engine.removed)
	}
}

func TestEditorCleanupCannotRaceGenerationCreation(t *testing.T) {
	backend, engine, i := cleanupFixture()
	engine.inspectErr = errdefs.ErrNotFound
	backend.starting.Store(backend.networkName(i), struct{}{})
	if err := backend.cleanupOrphan(context.Background(), i, backend.removeNetwork); err != nil || engine.disconnected || engine.removed {
		t.Fatalf("creation raced cleanup: %v", err)
	}
}
