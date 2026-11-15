package docker

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
	"github.com/moby/moby/api/pkg/stdcopy"
	client "github.com/moby/moby/client"
)

const (
	groupLabel           = "orkest.infrastructure.group"
	groupOwnerLabel      = "orkest.infrastructure.owner"
	groupInstallLabel    = "orkest.infrastructure.installation"
	groupGenerationLabel = "orkest.infrastructure.generation"
	groupMetadataLabel   = "orkest.infrastructure.metadata"
)

var _ engine.InfrastructureEngine = (*Engine)(nil)

func (e *Engine) ValidatePlan(ctx context.Context, plan engine.WorkloadPlan) error {
	if err := engine.ValidateWorkloadPlan(plan); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if plan.Group.InstallationID != "" && plan.Group.InstallationID != e.installation {
		return engine.ErrIdentityMismatch
	}
	if len(plan.Workloads) > 1 && len(plan.Networks) != 1 || len(plan.Workloads) == 1 && len(plan.Networks) > 1 || len(plan.Probes) != 0 {
		return engine.Unsupported("Docker generic workload group shape")
	}
	if len(plan.Workloads) > 1 || len(plan.Networks) == 1 {
		if e.networkClient == nil || len(plan.Networks) != 1 || plan.Networks[0].Existing != "" {
			return engine.Unsupported("Docker private workload network plan")
		}
		members := make(map[string]struct{}, len(plan.Networks[0].Members))
		for _, member := range plan.Networks[0].Members {
			members[member] = struct{}{}
		}
		if len(members) != len(plan.Workloads) {
			return fmt.Errorf("%w: private Docker network must include every workload", engine.ErrInvalidSpec)
		}
		for _, workload := range plan.Workloads {
			if _, ok := members[workload.ID]; !ok {
				return fmt.Errorf("%w: private Docker network omits workload %q", engine.ErrInvalidSpec, workload.ID)
			}
		}
	}
	for _, workload := range plan.Workloads {
		if workload.Mode != engine.ExecutionModeContainer {
			return engine.Unsupported("Docker native workloads")
		}
		if workload.WorkingPath != "" && !strings.HasPrefix(workload.WorkingPath, "/") {
			return fmt.Errorf("%w: container working directory must be absolute", engine.ErrInvalidSpec)
		}
		for _, item := range workload.Mounts {
			if item.SourceKind != engine.MountSourceHost && item.SourceKind != engine.MountSourceStorage {
				return engine.Unsupported("Docker generic mount source kind")
			}
			if !filepath.IsAbs(item.Source) {
				return fmt.Errorf("%w: Docker mount source must be absolute", engine.ErrInvalidSpec)
			}
		}
		for _, binding := range workload.Credentials {
			source, exists := e.config.CredentialSources[binding.Name]
			if !exists || source.FilePath == "" || !filepath.IsAbs(binding.Target) || binding.Target == "/" {
				return engine.Unsupported("Docker credential source is unavailable")
			}
		}
		for _, endpoint := range workload.Endpoints {
			if endpoint.ListenPort < 1 || endpoint.ListenHost != "" && endpoint.ListenHost != "0.0.0.0" {
				return fmt.Errorf("%w: Docker endpoints require a container port and wildcard bind", engine.ErrInvalidSpec)
			}
			if endpoint.ReadinessPath != "" {
				return engine.Unsupported("Docker generic endpoint readiness probes")
			}
		}
	}
	return nil
}

func (e *Engine) ResolveStorage(ctx context.Context, request engine.StorageRequest) ([]engine.StorageBinding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request.Owner == "" || request.Generation == "" {
		return nil, fmt.Errorf("%w: storage owner and generation are required", engine.ErrInvalidSpec)
	}
	bindings := make([]engine.StorageBinding, 0, len(request.Bindings))
	for _, requested := range request.Bindings {
		if requested.Name == "" {
			return nil, fmt.Errorf("%w: storage binding name is required", engine.ErrInvalidSpec)
		}
		identity := requested.StorageID
		var path string
		if identity == "" {
			if requested.Subpath != "" {
				return nil, fmt.Errorf("%w: storage subpath requires a stable storage identity", engine.ErrInvalidSpec)
			}
			identity = hashInfrastructureID(request.Owner + "\x00" + request.Generation + "\x00" + requested.Name)
			path = filepath.Join(e.config.DataRoot, "infrastructure-storage", identity)
		} else {
			var pathErr error
			path, pathErr = taskstorage.BindingPath(e.config.DataRoot, identity, requested.Subpath)
			if pathErr != nil {
				return nil, fmt.Errorf("%w: invalid Docker storage identity or subpath", engine.ErrInvalidSpec)
			}
		}
		if requested.Create {
			if err := os.MkdirAll(path, 0o700); err != nil {
				return nil, fmt.Errorf("create Docker host storage binding: %w", err)
			}
			if err := os.Chmod(path, 0o700); err != nil {
				return nil, fmt.Errorf("restrict Docker host storage binding: %w", err)
			}
		} else {
			info, err := os.Lstat(path)
			if err != nil {
				return nil, fmt.Errorf("inspect Docker host storage binding: %w", err)
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("%w: Docker host storage binding is not a regular directory", engine.ErrInvalidSpec)
			}
		}
		bindings = append(bindings, engine.StorageBinding{Name: requested.Name, Identity: identity, Subpath: requested.Subpath, HostPath: path})
	}
	return bindings, nil
}

func (e *Engine) StartPlan(ctx context.Context, plan engine.WorkloadPlan) (result engine.ResourceGroup, resultErr error) {
	if err := e.ValidatePlan(ctx, plan); err != nil {
		return engine.ResourceGroup{}, err
	}
	plan.Group.InstallationID = e.installation
	if err := e.manifests.Record(ctx, plan, "starting", nil); err != nil {
		return engine.ResourceGroup{}, err
	}
	defer func() {
		providerIDs := make(map[string]string, len(result.Components))
		for _, component := range result.Components {
			providerIDs[component.Ref.Key.ResourceID+"/"+component.Ref.Key.ComponentID] = component.Ref.ConcreteID
		}
		state := "uncertain"
		if resultErr == nil {
			state = "active"
		}
		if manifestErr := e.manifests.Record(context.WithoutCancel(ctx), plan, state, providerIDs); manifestErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("persist Docker operation result: %w", manifestErr))
		}
	}()
	return e.startInfrastructureWorkloadGroup(ctx, plan)
}

func (e *Engine) ensureInfrastructureNetwork(ctx context.Context, identity engine.GroupIdentity, plan engine.NetworkPlan) (string, error) {
	if e.networkClient == nil {
		return "", engine.Unsupported("Docker network lifecycle")
	}
	name := infrastructureNetworkName(identity)
	labels := map[string]string{
		groupLabel: name, groupOwnerLabel: identity.Owner,
		groupInstallLabel: e.installation, groupGenerationLabel: identity.Generation,
		"orkest.infrastructure.version":     "1",
		"orkest.infrastructure.network":     plan.ID,
		"orkest.infrastructure.deny-egress": strconv.FormatBool(plan.DenyEgress),
	}
	inspect, err := e.networkClient.NetworkInspect(ctx, name, client.NetworkInspectOptions{})
	if err == nil {
		if inspect.Network.Name != name || inspect.Network.Driver != "bridge" || inspect.Network.Internal != plan.DenyEgress || !sameLabels(inspect.Network.Labels, labels) {
			return "", engine.ErrIdentityMismatch
		}
	} else if !errdefs.IsNotFound(err) {
		return "", fmt.Errorf("inspect Docker infrastructure network: %w", err)
	} else {
		_, createErr := e.networkClient.NetworkCreate(ctx, name, client.NetworkCreateOptions{Driver: "bridge", Internal: plan.DenyEgress, Labels: labels})
		if createErr != nil {
			current, inspectErr := e.networkClient.NetworkInspect(ctx, name, client.NetworkInspectOptions{})
			if inspectErr != nil || current.Network.Name != name || current.Network.Driver != "bridge" || current.Network.Internal != plan.DenyEgress || !sameLabels(current.Network.Labels, labels) {
				return "", errors.Join(fmt.Errorf("create Docker infrastructure network: %w", createErr), engine.ErrUncertain, inspectErr)
			}
		}
	}
	if e.config.NetworkName != "" {
		hostname, hostErr := os.Hostname()
		if hostErr != nil {
			return "", fmt.Errorf("resolve Docker platform gateway: %w", hostErr)
		}
		gateway, inspectErr := e.networkClient.ContainerInspect(ctx, hostname, client.ContainerInspectOptions{})
		if inspectErr != nil {
			return "", fmt.Errorf("inspect Docker platform gateway: %w", inspectErr)
		}
		if gateway.Container.NetworkSettings == nil || gateway.Container.NetworkSettings.Networks[e.config.NetworkName] == nil {
			return "", engine.Unsupported("Docker platform gateway is not attached to its configured network")
		}
		if gateway.Container.NetworkSettings.Networks[name] == nil {
			if _, connectErr := e.networkClient.NetworkConnect(ctx, name, client.NetworkConnectOptions{Container: gateway.Container.ID}); connectErr != nil {
				return "", fmt.Errorf("attach Docker platform gateway to workload network: %w", connectErr)
			}
		}
	}
	return name, nil
}

func infrastructureNetworkName(identity engine.GroupIdentity) string {
	return "orkest-network-" + hashInfrastructureID(identity.InstallationID + "\x00" + identity.Owner + "\x00" + identity.Generation)[:24]
}

func sameLabels(actual, expected map[string]string) bool {
	for key, value := range expected {
		if actual[key] != value {
			return false
		}
	}
	return true
}

func setPlanMetadata(labels, metadata map[string]string) error {
	if len(metadata) == 0 {
		return nil
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode Docker workload metadata: %w", err)
	}
	if len(data) > 4096 {
		return fmt.Errorf("%w: workload metadata exceeds the Docker label limit", engine.ErrInvalidSpec)
	}
	labels[groupMetadataLabel] = base64.RawURLEncoding.EncodeToString(data)
	return nil
}

func readPlanMetadata(labels map[string]string) (map[string]string, error) {
	encoded := labels[groupMetadataLabel]
	if encoded == "" {
		return nil, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, engine.ErrUncertain
	}
	var metadata map[string]string
	if err = json.Unmarshal(data, &metadata); err != nil {
		return nil, engine.ErrUncertain
	}
	return metadata, nil
}

func (e *Engine) forwardInfrastructureLogs(containerID, owner, component string) {
	logs, err := e.client.ContainerLogs(context.Background(), containerID, client.ContainerLogsOptions{ShowStderr: true, Follow: true})
	if err != nil {
		return
	}
	defer func() { _ = logs.Close() }()
	reader, writer := io.Pipe()
	go func() {
		_, copyErr := stdcopy.StdCopy(io.Discard, writer, logs)
		_ = writer.CloseWithError(copyErr)
	}()
	engine.ForwardLogs(e.config.LogForwarder, reader, "group_owner", owner, "component", component)
}

func (e *Engine) InspectGroup(ctx context.Context, ref engine.GroupRef) (engine.GroupObservation, error) {
	if ref.Identity.InstallationID != "" && ref.Identity.InstallationID != e.installation {
		return engine.GroupObservation{}, engine.ErrIdentityMismatch
	}
	ref.Identity.InstallationID = e.installation
	return e.inspectWorkloadGroup(ctx, ref)
}

func (e *Engine) ListGroups(ctx context.Context, selector engine.Selector) ([]engine.ResourceGroup, error) {
	if selector.InstallationID != "" && selector.InstallationID != e.installation {
		return nil, engine.ErrIdentityMismatch
	}
	listed, err := e.networkClient.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: client.Filters{}.Add("label", groupInstallLabel+"="+e.installation)})
	if err != nil {
		return nil, fmt.Errorf("list Docker workload groups: %w", err)
	}
	groups := make([]engine.ResourceGroup, 0, len(listed.Items))
	seen := make(map[string]struct{}, len(listed.Items))
	for _, item := range listed.Items {
		labels := item.Labels
		if labels["orkest.infrastructure.group-mode"] != "workloads" || labels[groupLabel] == "" || selector.GroupOwner != "" && labels[groupOwnerLabel] != selector.GroupOwner {
			continue
		}
		identity := engine.GroupIdentity{InstallationID: e.installation, Owner: labels[groupOwnerLabel], Generation: labels[groupGenerationLabel]}
		key := identity.Owner + "\x00" + identity.Generation
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		observation, inspectErr := e.InspectGroup(ctx, engine.GroupRef{Identity: identity, Provider: workloadGroupProviderPrefix + infrastructureContainerName(identity)})
		if inspectErr != nil {
			return nil, inspectErr
		}
		if observation.State != engine.GroupAbsent {
			groups = append(groups, observation.Group)
		}
	}
	networks, err := e.networkClient.NetworkList(ctx, client.NetworkListOptions{Filters: client.Filters{}.Add("label", groupInstallLabel+"="+e.installation)})
	if err != nil {
		return nil, fmt.Errorf("list Docker infrastructure networks: %w", err)
	}
	for _, item := range networks.Items {
		labels := item.Labels
		if labels[groupLabel] == "" || selector.GroupOwner != "" && labels[groupOwnerLabel] != selector.GroupOwner {
			continue
		}
		identity := engine.GroupIdentity{InstallationID: e.installation, Owner: labels[groupOwnerLabel], Generation: labels[groupGenerationLabel]}
		key := identity.Owner + "\x00" + identity.Generation
		if _, exists := seen[key]; exists {
			continue
		}
		if item.Name != infrastructureNetworkName(identity) || identity.Owner == "" || identity.Generation == "" {
			return nil, engine.ErrIdentityMismatch
		}
		seen[key] = struct{}{}
		observation, inspectErr := e.InspectGroup(ctx, engine.GroupRef{Identity: identity, Provider: workloadGroupProviderPrefix + infrastructureContainerName(identity)})
		if inspectErr != nil {
			return nil, inspectErr
		}
		if observation.State != engine.GroupAbsent {
			groups = append(groups, observation.Group)
		}
	}
	return groups, nil
}

func (e *Engine) StopGroup(ctx context.Context, ref engine.GroupRef) (resultErr error) {
	defer func() {
		if resultErr == nil {
			resultErr = e.manifests.Update(context.WithoutCancel(ctx), ref.Identity, "stopped", nil)
		}
	}()
	observation, err := e.InspectGroup(ctx, ref)
	if err != nil || observation.State == engine.GroupAbsent || observation.State == engine.GroupStopped {
		return err
	}
	if err = e.stopWorkloadGroup(ctx, observation.Group); err != nil {
		return err
	}
	return e.waitForWorkloadGroupStop(ctx, ref)
}

func (e *Engine) CleanupGroup(ctx context.Context, ref engine.GroupRef) (resultErr error) {
	defer func() {
		if resultErr == nil {
			resultErr = e.manifests.Update(context.WithoutCancel(ctx), ref.Identity, "cleaned", nil)
		}
	}()
	observation, err := e.InspectGroup(ctx, ref)
	if err != nil || observation.State == engine.GroupAbsent {
		return err
	}
	if observation.State != engine.GroupStopped {
		return errors.New("refusing to remove active or uncertain Docker workload group")
	}
	return e.cleanupWorkloadGroup(ctx, observation.Group, ref)
}

func (e *Engine) inspectInfrastructureNetwork(ctx context.Context, identity engine.GroupIdentity) (bool, error) {
	if e.networkClient == nil {
		return false, nil
	}
	name := infrastructureNetworkName(identity)
	network, err := e.networkClient.NetworkInspect(ctx, name, client.NetworkInspectOptions{})
	if errdefs.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect Docker infrastructure network: %w", err)
	}
	labels := network.Network.Labels
	denyEgress, parseErr := strconv.ParseBool(labels["orkest.infrastructure.deny-egress"])
	if network.Network.Name != name || network.Network.Driver != "bridge" || parseErr != nil || labels[groupLabel] != name || labels[groupInstallLabel] != e.installation || labels[groupOwnerLabel] != identity.Owner || labels[groupGenerationLabel] != identity.Generation || denyEgress != network.Network.Internal {
		return false, engine.ErrIdentityMismatch
	}
	return true, nil
}

func (e *Engine) removeInfrastructureNetwork(ctx context.Context, identity engine.GroupIdentity) error {
	if e.networkClient == nil {
		return nil
	}
	name := infrastructureNetworkName(identity)
	if _, err := e.inspectInfrastructureNetwork(ctx, identity); err != nil {
		return err
	}
	network, err := e.networkClient.NetworkInspect(ctx, name, client.NetworkInspectOptions{})
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect Docker infrastructure network for cleanup: %w", err)
	}
	for containerID := range network.Network.Containers {
		if _, err = e.networkClient.NetworkDisconnect(ctx, network.Network.ID, client.NetworkDisconnectOptions{Container: containerID, Force: true}); err != nil && !errdefs.IsNotFound(err) {
			return fmt.Errorf("disconnect Docker infrastructure network member: %w", err)
		}
	}
	if _, err = e.networkClient.NetworkRemove(ctx, network.Network.ID, client.NetworkRemoveOptions{}); err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("remove Docker infrastructure network: %w", err)
	}
	return nil
}

func infrastructureContainerName(identity engine.GroupIdentity) string {
	return "orkest-workload-" + hashInfrastructureID(identity.InstallationID + "\x00" + identity.Owner + "\x00" + identity.Generation)[:24]
}

func hashInfrastructureID(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func endpointByName(endpoints []engine.Endpoint, name string) (engine.Endpoint, bool) {
	for _, endpoint := range endpoints {
		if endpoint.Name == name {
			return endpoint, true
		}
	}
	return engine.Endpoint{}, false
}

func ensureInfrastructureMountTarget(dataRoot, groupName, target string) error {
	return ensureGenericMountTarget(filepath.Join(dataRoot, "infrastructure-mount-targets", groupName), target, true)
}

func ensureGenericMountTarget(root, target string, directory bool) error {
	clean := filepath.Clean(target)
	if !filepath.IsAbs(clean) || clean == string(filepath.Separator) {
		return fmt.Errorf("%w: storage mount target must be an absolute non-root path", engine.ErrInvalidSpec)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("prepare Docker generic mount root: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return fmt.Errorf("restrict Docker storage mount root: %w", err)
	}
	local, err := genericMountTarget(root, clean)
	if err != nil {
		return err
	}
	if directory {
		err = os.MkdirAll(local, 0o700)
	} else {
		err = os.MkdirAll(filepath.Dir(local), 0o700)
		if err == nil {
			file, createErr := os.OpenFile(local, os.O_CREATE|os.O_WRONLY, 0o600)
			if createErr == nil {
				createErr = file.Close()
			}
			err = createErr
		}
	}
	if err != nil {
		return fmt.Errorf("prepare Docker storage mount target: %w", err)
	}
	if !pathWithin(root, local) {
		return fmt.Errorf("%w: storage mount target escapes the engine mount root", engine.ErrInvalidSpec)
	}
	actual, err := filepath.EvalSymlinks(local)
	if err != nil {
		return fmt.Errorf("resolve Docker storage mount target: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve Docker storage mount root: %w", err)
	}
	if !pathWithin(resolvedRoot, actual) {
		return fmt.Errorf("%w: storage mount target resolves outside the engine mount root", engine.ErrInvalidSpec)
	}
	return nil
}

func genericMountTarget(root, target string) (string, error) {
	clean := filepath.Clean(target)
	if !filepath.IsAbs(clean) || clean == string(filepath.Separator) {
		return "", fmt.Errorf("%w: mount target must be an absolute non-root path", engine.ErrInvalidSpec)
	}
	local := filepath.Join(root, strings.TrimPrefix(clean, string(filepath.Separator)))
	if !pathWithin(root, local) {
		return "", fmt.Errorf("%w: mount target escapes the engine mount root", engine.ErrInvalidSpec)
	}
	return local, nil
}

func pathWithin(root, value string) bool {
	relative, err := filepath.Rel(root, value)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
