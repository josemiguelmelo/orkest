package docker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	client "github.com/moby/moby/client"
)

const editorRelayTokenEnv = "ORKEST_EDITOR_RELAY_TOKEN"

func (b *EditorBackend) relayName(i taskeditor.Instance) string { return b.networkName(i) + "-relay" }

// Native host gateways cannot publish ports from internal-only Docker networks.
// This small fixed-upstream relay joins both networks, without granting the
// editor shell any external route. Every ingress request needs a private token.
func (b *EditorBackend) startRelay(ctx context.Context, i taskeditor.Instance) error {
	token := make([]byte, 32)
	_, _ = rand.Read(token)
	labels := b.labels(i)
	labels[editorLabel+".kind"] = "relay"
	port := network.MustParsePort("8081/tcp")
	pids := int64(32)
	created, err := b.engine.ContainerCreate(ctx, client.ContainerCreateOptions{Name: b.relayName(i), Config: &container.Config{Image: b.config.Image, User: b.config.ContainerUser, Entrypoint: []string{"/opt/code-server/lib/node", "/opt/editor/relay.cjs"}, Env: []string{editorRelayTokenEnv + "=" + hex.EncodeToString(token), "ORKEST_EDITOR_RELAY_UPSTREAM=http://" + b.name(i.Scope) + ":8080"}, Labels: labels, ExposedPorts: network.PortSet{port: {}}}, HostConfig: &container.HostConfig{NetworkMode: "bridge", ReadonlyRootfs: true, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"}, Resources: container.Resources{Memory: 128 << 20, NanoCPUs: 1e9, PidsLimit: &pids}, PortBindings: network.PortMap{port: {{HostIP: netip.MustParseAddr("127.0.0.1")}}}, RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyDisabled}}})
	if err != nil {
		return fmt.Errorf("create editor relay: %w", err)
	}
	if _, err = b.engine.NetworkConnect(ctx, b.networkName(i), client.NetworkConnectOptions{Container: created.ID}); err != nil {
		return err
	}
	_, err = b.engine.ContainerStart(ctx, created.ID, client.ContainerStartOptions{})
	return err
}

func (b *EditorBackend) inspectRelay(ctx context.Context, i taskeditor.Instance) (taskeditor.Instance, bool, error) {
	result, err := b.engine.ContainerInspect(ctx, b.relayName(i), client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		return i, true, nil
	}
	if err != nil {
		return i, true, err
	}
	actual, err := b.decode(result.Container)
	if err != nil {
		return i, true, err
	}
	if actual.Scope != i.Scope || actual.Generation != i.Generation || actual.UserID != i.UserID || result.Container.Config.Labels[editorLabel+".kind"] != "relay" {
		return i, true, errors.New("editor relay ownership mismatch")
	}
	if result.Container.State == nil || !result.Container.State.Running {
		return i, true, nil
	}
	for _, variable := range result.Container.Config.Env {
		if value, found := strings.CutPrefix(variable, editorRelayTokenEnv+"="); found {
			i.GatewayToken = value
		}
	}
	if len(i.GatewayToken) != 64 {
		return i, true, errors.New("editor relay credentials unavailable")
	}
	i.Endpoint, err = publishedEndpoint(result.Container, 8081)
	return i, true, err
}

func (b *EditorBackend) stopRelay(ctx context.Context, i taskeditor.Instance) error {
	result, err := b.engine.ContainerInspect(ctx, b.relayName(i), client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	actual, err := b.decode(result.Container)
	if err != nil {
		return err
	}
	if actual.Scope != i.Scope || actual.Generation != i.Generation || actual.UserID != i.UserID || result.Container.Config.Labels[editorLabel+".kind"] != "relay" {
		return errors.New("editor relay stop identity mismatch")
	}
	timeout := 3
	if _, err = b.engine.ContainerStop(ctx, result.Container.ID, client.ContainerStopOptions{Timeout: &timeout}); err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	// Removing without Force refuses a still-running relay.
	_, err = b.engine.ContainerRemove(ctx, result.Container.ID, client.ContainerRemoveOptions{})
	if errdefs.IsNotFound(err) {
		return nil
	}
	return err
}

func (b *EditorBackend) cleanupOrphanRelays(ctx context.Context, containers []container.Summary, instances []taskeditor.Instance) error {
	for _, summary := range containers {
		if summary.Labels[editorLabel+".kind"] != "relay" {
			continue
		}
		i, err := b.decode(container.InspectResponse{ID: summary.ID, Config: &container.Config{Labels: summary.Labels}})
		if err != nil {
			return err
		}
		if _, starting := b.starting.Load(b.networkName(i)); starting {
			continue
		}
		owned := false
		for _, instance := range instances {
			if instance.Scope == i.Scope && instance.Generation == i.Generation {
				owned = true
				break
			}
		}
		if !owned {
			if err = b.cleanupOrphan(ctx, i, b.stopRelay); err != nil {
				return err
			}
		}
	}
	return nil
}
