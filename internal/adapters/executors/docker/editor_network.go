package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/containerd/errdefs"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
	"github.com/moby/moby/api/types/container"
	client "github.com/moby/moby/client"
)

func (b *EditorBackend) networkName(i taskeditor.Instance) string {
	digest := sha256.Sum256([]byte(b.name(i.Scope) + ":" + i.Generation))
	return "orkest-editor-net-" + hex.EncodeToString(digest[:])[:48]
}

// Only the trusted gateway joins an editor's network. Editors never join the
// worker control network, and different editor generations never share a bridge.
func (b *EditorBackend) createNetwork(ctx context.Context, i taskeditor.Instance) error {
	gateway := ""
	if b.executor.config.NetworkName != "" {
		hostname, err := os.Hostname()
		if err != nil {
			return err
		}
		inspected, err := b.engine.ContainerInspect(ctx, hostname, client.ContainerInspectOptions{})
		if err != nil {
			return fmt.Errorf("resolve containerized editor gateway: %w", err)
		}
		if inspected.Container.NetworkSettings == nil || inspected.Container.NetworkSettings.Networks[b.executor.config.NetworkName] == nil {
			return errors.New("editor gateway is not attached to the configured platform network")
		}
		gateway = inspected.Container.ID
	}
	labels := b.labels(i)
	labels[editorLabel+".gateway"] = gateway
	created, err := b.engine.NetworkCreate(ctx, b.networkName(i), client.NetworkCreateOptions{Driver: "bridge", Internal: true, Labels: labels})
	if err != nil {
		return fmt.Errorf("create private editor network: %w", err)
	}
	if gateway != "" {
		if _, err = b.engine.NetworkConnect(ctx, created.ID, client.NetworkConnectOptions{Container: gateway}); err != nil {
			return fmt.Errorf("attach trusted editor gateway: %w", err)
		}
	}
	return nil
}

func (b *EditorBackend) removeNetwork(ctx context.Context, i taskeditor.Instance) error {
	inspected, err := b.engine.NetworkInspect(ctx, b.networkName(i), client.NetworkInspectOptions{})
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	actual, err := b.decode(container.InspectResponse{ID: inspected.Network.ID, Config: &container.Config{Labels: inspected.Network.Labels}})
	if err != nil {
		return err
	}
	if actual.Scope != i.Scope || actual.Generation != i.Generation || actual.UserID != i.UserID || inspected.Network.Name != b.networkName(i) || inspected.Network.Driver != "bridge" || !inspected.Network.Internal {
		return errors.New("editor network ownership mismatch")
	}
	gateway := inspected.Network.Labels[editorLabel+".gateway"]
	if _, attached := inspected.Network.Containers[gateway]; gateway != "" && attached {
		if _, err = b.engine.NetworkDisconnect(ctx, inspected.Network.ID, client.NetworkDisconnectOptions{Container: gateway}); err != nil && !errdefs.IsNotFound(err) {
			return err
		}
	}
	_, err = b.engine.NetworkRemove(ctx, inspected.Network.ID, client.NetworkRemoveOptions{})
	if errdefs.IsNotFound(err) {
		return nil
	}
	return err
}

// A lost create response may leave a container. Do not remove its network until
// inspect establishes absence; startup discovery will reconcile uncertain cases.
func (b *EditorBackend) cleanupUncreatedNetwork(ctx context.Context, i taskeditor.Instance) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	_, err := b.engine.ContainerInspect(cleanupCtx, b.name(i.Scope), client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		_ = b.removeNetwork(cleanupCtx, i)
	}
}

func (b *EditorBackend) cleanupOrphanNetworks(ctx context.Context, instances []taskeditor.Instance) error {
	listed, err := b.engine.NetworkList(ctx, client.NetworkListOptions{Filters: client.Filters{}.Add("label", editorLabel+"="+b.installation)})
	if err != nil {
		return err
	}
	for _, network := range listed.Items {
		if _, starting := b.starting.Load(network.Name); starting {
			continue
		}
		i, err := b.decode(container.InspectResponse{ID: network.ID, Config: &container.Config{Labels: network.Labels}})
		if err != nil {
			return err
		}
		if network.Name != b.networkName(i) {
			return errors.New("unexpected managed editor network name")
		}
		owned := false
		for _, instance := range instances {
			if instance.Scope == i.Scope && instance.Generation == i.Generation {
				owned = true
				break
			}
		}
		if !owned {
			if err = b.removeNetwork(ctx, i); err != nil {
				return err
			}
		}
	}
	return nil
}
