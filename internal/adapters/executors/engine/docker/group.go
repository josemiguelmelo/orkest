package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	client "github.com/moby/moby/client"
)

const workloadGroupProviderPrefix = "orkest-workload-group-v1:"

func (e *Engine) startInfrastructureWorkloadGroup(ctx context.Context, plan engine.WorkloadPlan) (engine.ResourceGroup, error) {
	plan.Group.InstallationID = e.installation
	groupName := infrastructureContainerName(plan.Group)
	ref := engine.GroupRef{Identity: plan.Group, Provider: workloadGroupProviderPrefix + groupName}
	prior, err := e.InspectGroup(ctx, engine.GroupRef{Identity: plan.Group})
	if err != nil {
		return engine.ResourceGroup{}, fmt.Errorf("reconcile Docker workload group before create: %w", err)
	}
	if prior.State != engine.GroupAbsent {
		return prior.Group, engine.ErrUncertain
	}
	networkPlan := engine.NetworkPlan{ID: "workloads", DenyEgress: false}
	if len(plan.Networks) != 0 {
		networkPlan = plan.Networks[0]
	} else {
		for _, workload := range plan.Workloads {
			networkPlan.Members = append(networkPlan.Members, workload.ID)
		}
	}
	networkName, err := e.ensureInfrastructureNetwork(ctx, plan.Group, networkPlan)
	if err != nil {
		return engine.ResourceGroup{Ref: ref}, errors.Join(err, engine.ErrUncertain)
	}
	storageRequests := make([]engine.StorageBindingRequest, 0, len(plan.Storage))
	for _, attachment := range plan.Storage {
		storageRequests = append(storageRequests, engine.StorageBindingRequest{
			Name: attachment.WorkloadID + "-" + attachment.Binding, StorageID: attachment.StorageID,
			Subpath: attachment.Subpath, Durable: attachment.Durable, Create: attachment.Create, PlatformIO: attachment.PlatformIO,
		})
	}
	bindings, err := e.ResolveStorage(ctx, engine.StorageRequest{Owner: plan.Group.Owner, Generation: plan.Group.Generation, Bindings: storageRequests})
	if err != nil {
		return engine.ResourceGroup{Ref: ref}, err
	}
	storageByName := make(map[string]engine.StorageBinding, len(bindings))
	for _, binding := range bindings {
		storageByName[binding.Name] = binding
	}
	workloadNames := make(map[string]string, len(plan.Workloads))
	for _, workload := range plan.Workloads {
		workloadNames[workload.ID] = infrastructureWorkloadName(plan.Group, workload.ID)
	}
	for _, workload := range plan.Workloads {
		if err = ctx.Err(); err != nil {
			return engine.ResourceGroup{Ref: ref}, errors.Join(err, engine.ErrUncertain)
		}
		options, optionErr := e.infrastructureContainerOptions(plan, workload, groupName, networkName, workloadNames, storageByName)
		if optionErr != nil {
			return engine.ResourceGroup{Ref: ref}, errors.Join(optionErr, engine.ErrUncertain)
		}
		created, createErr := e.networkClient.ContainerCreate(ctx, options)
		if createErr != nil || created.ID == "" {
			if createErr == nil {
				createErr = engine.ErrUncertain
			}
			return engine.ResourceGroup{Ref: ref}, errors.Join(fmt.Errorf("create Docker workload %q: %w", workload.ID, createErr), engine.ErrUncertain)
		}
	}
	for _, workload := range plan.Workloads {
		name := workloadNames[workload.ID]
		if _, err = e.networkClient.ContainerStart(ctx, name, client.ContainerStartOptions{}); err != nil {
			return engine.ResourceGroup{Ref: ref}, errors.Join(fmt.Errorf("start Docker workload %q: %w", workload.ID, err), engine.ErrUncertain)
		}
	}
	observation, err := e.inspectWorkloadGroup(ctx, ref)
	if err != nil {
		return observation.Group, errors.Join(fmt.Errorf("inspect started Docker workload group: %w", err), engine.ErrUncertain)
	}
	if observation.State != engine.GroupActive {
		return observation.Group, engine.ErrUncertain
	}
	for _, workload := range plan.Workloads {
		go e.forwardInfrastructureLogs(workloadNames[workload.ID], plan.Group.Owner, workload.ID)
	}
	return observation.Group, nil
}

func (e *Engine) infrastructureContainerOptions(plan engine.WorkloadPlan, workload engine.Workload, groupName, networkName string, workloadNames map[string]string, storage map[string]engine.StorageBinding) (client.ContainerCreateOptions, error) {
	name := workloadNames[workload.ID]
	labels := map[string]string{
		groupLabel: groupName, groupOwnerLabel: plan.Group.Owner,
		groupInstallLabel: e.installation, groupGenerationLabel: plan.Group.Generation,
		"orkest.infrastructure.version":    "1",
		"orkest.infrastructure.component":  workload.ID,
		"orkest.infrastructure.group-mode": "workloads",
	}
	if err := setPlanMetadata(labels, plan.Metadata); err != nil {
		return client.ContainerCreateOptions{}, err
	}
	for endpointName, address := range workloadEndpointMetadata(workload) {
		labels["orkest.infrastructure.endpoint."+endpointName] = address
	}
	variables := make([]string, 0, len(workload.Environment)+len(workload.Credentials))
	for key, value := range workload.Environment {
		if strings.TrimSpace(key) == "" || strings.Contains(key, "=") {
			return client.ContainerCreateOptions{}, fmt.Errorf("%w: invalid workload environment name", engine.ErrInvalidSpec)
		}
		variables = append(variables, key+"="+value)
	}
	for key, reference := range workload.EnvironmentEndpointRefs {
		targetWorkload := reference.Workload
		if targetWorkload == "" {
			targetWorkload = workload.ID
		}
		target := workloadByID(plan.Workloads, targetWorkload)
		endpoint, ok := endpointByName(target.Endpoints, reference.Endpoint)
		if !ok {
			return client.ContainerCreateOptions{}, fmt.Errorf("%w: unknown endpoint environment reference", engine.ErrInvalidSpec)
		}
		address := net.JoinHostPort(workloadNames[targetWorkload], fmt.Sprintf("%d", endpoint.ListenPort))
		if targetWorkload == workload.ID {
			host := endpoint.ListenHost
			if host == "" {
				host = "0.0.0.0"
			}
			address = net.JoinHostPort(host, fmt.Sprintf("%d", endpoint.ListenPort))
		} else if endpoint.Protocol == "http" || endpoint.Protocol == "https" {
			address = endpoint.Protocol + "://" + address
		}
		variables = append(variables, key+"="+address)
	}
	groupTargetRoot := filepath.Join(e.config.DataRoot, "infrastructure-mount-targets", groupName, hashInfrastructureID(workload.ID)[:12])
	mounts := make([]mount.Mount, 0, len(workload.Credentials)+len(workload.Mounts)+len(plan.Storage)+len(plan.Files))
	for _, attachment := range plan.Storage {
		if attachment.WorkloadID != workload.ID {
			continue
		}
		binding, exists := storage[workload.ID+"-"+attachment.Binding]
		if !exists {
			return client.ContainerCreateOptions{}, fmt.Errorf("%w: storage binding %q was not resolved", engine.ErrInvalidSpec, attachment.Binding)
		}
		if err := ensureInfrastructureMountTarget(e.config.DataRoot, groupName, attachment.Target); err != nil {
			return client.ContainerCreateOptions{}, err
		}
		mounts = append(mounts, mount.Mount{Type: mount.TypeBind, Source: binding.HostPath, Target: attachment.Target, ReadOnly: attachment.ReadOnly})
	}
	for _, item := range workload.Mounts {
		info, err := os.Lstat(item.Source)
		if err != nil {
			return client.ContainerCreateOptions{}, fmt.Errorf("inspect Docker mount source: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() && !info.Mode().IsRegular() {
			return client.ContainerCreateOptions{}, fmt.Errorf("%w: Docker mount source must be a regular file or directory without symlinks", engine.ErrInvalidSpec)
		}
		if err = ensureGenericMountTarget(groupTargetRoot, item.Target, info.IsDir()); err != nil {
			return client.ContainerCreateOptions{}, err
		}
		mounts = append(mounts, mount.Mount{Type: mount.TypeBind, Source: item.Source, Target: item.Target, ReadOnly: item.ReadOnly})
	}
	for _, file := range plan.Files {
		targetWorkload := file.WorkloadID
		if targetWorkload == "" && len(plan.Workloads) == 1 {
			targetWorkload = workload.ID
		}
		if targetWorkload != workload.ID {
			continue
		}
		target, err := genericMountTarget(groupTargetRoot, file.Target)
		if err != nil {
			return client.ContainerCreateOptions{}, err
		}
		if err = os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return client.ContainerCreateOptions{}, fmt.Errorf("prepare Docker generated file parent: %w", err)
		}
		if err = os.WriteFile(target, file.Contents, os.FileMode(file.Mode)); err != nil {
			return client.ContainerCreateOptions{}, fmt.Errorf("materialize Docker generated file: %w", err)
		}
		if err = os.Chmod(target, os.FileMode(file.Mode)); err != nil {
			return client.ContainerCreateOptions{}, fmt.Errorf("restrict Docker generated file: %w", err)
		}
		mounts = append(mounts, mount.Mount{Type: mount.TypeBind, Source: target, Target: file.Target, ReadOnly: true})
	}
	for _, binding := range workload.Credentials {
		source := e.config.CredentialSources[binding.Name]
		if err := ensureCredentialMountTarget(groupTargetRoot, binding.Target); err != nil {
			return client.ContainerCreateOptions{}, fmt.Errorf("prepare Docker credential target: %w", err)
		}
		mounts = append(mounts, mount.Mount{Type: mount.TypeBind, Source: source.FilePath, Target: binding.Target, ReadOnly: binding.ReadOnly})
		if binding.EnvironmentName != "" {
			variables = append(variables, binding.EnvironmentName+"="+binding.Target)
		}
	}
	sort.Strings(variables)
	command := make([]string, 0, len(workload.Arguments))
	for _, argument := range workload.Arguments {
		if argument.Kind == engine.ArgumentLiteral {
			command = append(command, argument.Value)
			continue
		}
		targetWorkload := argument.Workload
		if targetWorkload == "" {
			targetWorkload = workload.ID
		}
		endpoint, ok := endpointByName(workloadByID(plan.Workloads, targetWorkload).Endpoints, argument.Endpoint)
		if !ok {
			return client.ContainerCreateOptions{}, fmt.Errorf("%w: unknown workload endpoint %q", engine.ErrInvalidSpec, argument.Endpoint)
		}
		address := net.JoinHostPort(workloadNames[targetWorkload], fmt.Sprintf("%d", endpoint.ListenPort))
		if targetWorkload == workload.ID {
			host := endpoint.ListenHost
			if host == "" {
				host = "0.0.0.0"
			}
			address = net.JoinHostPort(host, fmt.Sprintf("%d", endpoint.ListenPort))
		} else if endpoint.Protocol == "http" || endpoint.Protocol == "https" {
			address = endpoint.Protocol + "://" + address
		}
		command = append(command, address)
	}
	if len(command) == 0 {
		command = workload.Artifact.ContainerCommand
	}
	exposed := network.PortSet{}
	portBindings := network.PortMap{}
	for _, endpoint := range workload.Endpoints {
		transport := endpoint.Protocol
		if transport == "http" || transport == "https" {
			transport = "tcp"
		}
		port := network.MustParsePort(fmt.Sprintf("%d/%s", endpoint.ListenPort, transport))
		exposed[port] = struct{}{}
		if endpoint.Exposure == engine.ExposurePlatform && e.config.NetworkName == "" {
			portBindings[port] = []network.PortBinding{{HostIP: netip.MustParseAddr("127.0.0.1")}}
		}
	}
	user, err := e.containerUser(workload.Security)
	if err != nil {
		return client.ContainerCreateOptions{}, err
	}
	tmpfs := make(map[string]string, len(workload.TemporaryMounts))
	for _, item := range workload.TemporaryMounts {
		tmpfs[item.Target] = tmpfsOptions(item)
	}
	resources := container.Resources{Memory: workload.Limits.MemoryBytes, NanoCPUs: workload.Limits.NanoCPUs}
	if workload.Limits.ProcessCount > 0 {
		count := workload.Limits.ProcessCount
		resources.PidsLimit = &count
	}
	capabilities, securityOptions := []string(nil), []string(nil)
	if workload.Security.DropAllCapabilities {
		capabilities = []string{"ALL"}
	}
	if workload.Security.NoPrivilegeEscalation {
		securityOptions = []string{"no-new-privileges:true"}
	}
	restart := container.RestartPolicyDisabled
	if workload.Restart == engine.RestartOnFailure {
		restart = container.RestartPolicyOnFailure
	}
	return client.ContainerCreateOptions{
		Name: name,
		Config: &container.Config{
			Image: workload.Artifact.ContainerImage, User: user, Entrypoint: workload.Artifact.ContainerEntrypoint,
			Cmd: command, Env: variables, WorkingDir: workload.WorkingPath, Labels: labels, ExposedPorts: exposed,
		},
		HostConfig: &container.HostConfig{
			Mounts: mounts, NetworkMode: container.NetworkMode(networkName), PortBindings: portBindings, RestartPolicy: container.RestartPolicy{Name: restart},
			ReadonlyRootfs: workload.Security.ReadOnlyRoot, CapDrop: capabilities, SecurityOpt: securityOptions,
			Tmpfs: tmpfs, Resources: resources,
		},
		NetworkingConfig: &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{networkName: {Aliases: []string{workload.ID}}}},
	}, nil
}

func (e *Engine) inspectWorkloadGroup(ctx context.Context, ref engine.GroupRef) (engine.GroupObservation, error) {
	items, err := e.networkClient.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: client.Filters{}.Add("label", groupLabel+"="+infrastructureContainerName(ref.Identity))})
	if err != nil {
		return engine.GroupObservation{}, fmt.Errorf("list Docker workload group components: %w", err)
	}
	group := engine.ResourceGroup{Ref: ref, Endpoints: make(map[string]string)}
	if len(items.Items) == 0 {
		networkExists, networkErr := e.inspectInfrastructureNetwork(ctx, ref.Identity)
		if networkErr != nil {
			return engine.GroupObservation{}, networkErr
		}
		if networkExists {
			return engine.GroupObservation{State: engine.GroupStopped, Group: group}, nil
		}
		return engine.GroupObservation{State: engine.GroupAbsent}, nil
	}
	endpointValues := make(map[string]map[string]string, len(items.Items))
	var metadata map[string]string
	active, stopped := 0, 0
	for _, item := range items.Items {
		inspection, inspectErr := e.networkClient.ContainerInspect(ctx, item.ID, client.ContainerInspectOptions{})
		if errdefs.IsNotFound(inspectErr) {
			continue
		}
		if inspectErr != nil {
			return engine.GroupObservation{}, fmt.Errorf("inspect Docker workload group component: %w", inspectErr)
		}
		labels := inspection.Container.Config.Labels
		if labels[groupLabel] != infrastructureContainerName(ref.Identity) || labels[groupOwnerLabel] != ref.Identity.Owner || labels[groupGenerationLabel] != ref.Identity.Generation || labels[groupInstallLabel] != e.installation || labels["orkest.infrastructure.group-mode"] != "workloads" {
			return engine.GroupObservation{}, engine.ErrIdentityMismatch
		}
		if inspection.Container.State == nil {
			return engine.GroupObservation{State: engine.GroupUncertain}, nil
		}
		state := engine.ResourceStopped
		if inspection.Container.State.Running || inspection.Container.State.Restarting {
			state = engine.ResourceRunning
			active++
		} else {
			stopped++
		}
		component := labels["orkest.infrastructure.component"]
		if component == "" {
			return engine.GroupObservation{}, engine.ErrIdentityMismatch
		}
		componentMetadata, metadataErr := readPlanMetadata(labels)
		if metadataErr != nil {
			return engine.GroupObservation{}, metadataErr
		}
		if metadata == nil {
			metadata = componentMetadata
		} else if !reflect.DeepEqual(metadata, componentMetadata) {
			return engine.GroupObservation{}, engine.ErrIdentityMismatch
		}
		resource := engine.Resource{Ref: engine.ResourceRef{Key: engine.ResourceKey{InstallationID: e.installation, ResourceID: labels[groupLabel], ComponentID: component}, ConcreteID: inspection.Container.ID}, State: state}
		group.Components = append(group.Components, resource)
		componentEndpoints := make(map[string]string)
		for label, value := range labels {
			endpointName, found := strings.CutPrefix(label, "orkest.infrastructure.endpoint.")
			if !found {
				continue
			}
			var endpoint engine.Endpoint
			if err = json.Unmarshal([]byte(value), &endpoint); err != nil {
				return engine.GroupObservation{}, engine.ErrUncertain
			}
			address := net.JoinHostPort(workloadNamesFromGroup(ref.Identity, component), fmt.Sprintf("%d", endpoint.ListenPort))
			if endpoint.Exposure == engine.ExposurePlatform && e.config.NetworkName == "" {
				address, err = publishedEndpoint(inspection.Container, endpoint.ListenPort)
				if err != nil {
					return engine.GroupObservation{}, err
				}
			} else if endpoint.Protocol == "http" || endpoint.Protocol == "https" {
				address = endpoint.Protocol + "://" + address
			}
			componentEndpoints[endpointName] = address
		}
		endpointValues[component] = componentEndpoints
	}
	if len(group.Components) == 0 {
		return engine.GroupObservation{State: engine.GroupAbsent}, nil
	}
	group.Metadata = metadata
	sort.Slice(group.Components, func(i, j int) bool {
		return group.Components[i].Ref.Key.ComponentID < group.Components[j].Ref.Key.ComponentID
	})
	endpointCounts := make(map[string]int)
	for _, endpoints := range endpointValues {
		for endpoint := range endpoints {
			endpointCounts[endpoint]++
		}
	}
	for component, endpoints := range endpointValues {
		for endpoint, address := range endpoints {
			group.Endpoints[component+"."+endpoint] = address
			if endpointCounts[endpoint] == 1 {
				group.Endpoints[endpoint] = address
			}
		}
	}
	manifest, manifestErr := e.manifests.Read(ctx, ref.Identity)
	if manifestErr != nil {
		return engine.GroupObservation{State: engine.GroupUncertain, Group: group}, nil
	}
	if len(manifest.Components) != len(group.Components) {
		return engine.GroupObservation{State: engine.GroupUncertain, Group: group}, nil
	}
	observed := make(map[string]struct{}, len(group.Components))
	for _, component := range group.Components {
		observed[component.Ref.Key.ComponentID] = struct{}{}
	}
	for _, expected := range manifest.Components {
		if _, exists := observed[expected]; !exists {
			return engine.GroupObservation{State: engine.GroupUncertain, Group: group}, nil
		}
	}
	if active == len(group.Components) {
		return engine.GroupObservation{State: engine.GroupActive, Group: group}, nil
	}
	if stopped == len(group.Components) {
		return engine.GroupObservation{State: engine.GroupStopped, Group: group}, nil
	}
	return engine.GroupObservation{State: engine.GroupUncertain, Group: group}, nil
}

func (e *Engine) stopWorkloadGroup(ctx context.Context, group engine.ResourceGroup) error {
	for _, component := range group.Components {
		if component.State != engine.ResourceRunning {
			continue
		}
		_, err := e.networkClient.ContainerStop(ctx, component.Ref.ConcreteID, client.ContainerStopOptions{})
		if err != nil && !errdefs.IsNotFound(err) {
			return fmt.Errorf("stop Docker workload component %q: %w", component.Ref.Key.ComponentID, err)
		}
	}
	return nil
}

func (e *Engine) waitForWorkloadGroupStop(ctx context.Context, ref engine.GroupRef) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		observation, err := e.inspectWorkloadGroup(ctx, ref)
		if err != nil {
			return err
		}
		if observation.State == engine.GroupAbsent || observation.State == engine.GroupStopped {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), engine.ErrUncertain)
		case <-ticker.C:
		}
	}
}

func (e *Engine) cleanupWorkloadGroup(ctx context.Context, group engine.ResourceGroup, ref engine.GroupRef) error {
	for _, component := range group.Components {
		if _, err := e.networkClient.ContainerRemove(ctx, component.Ref.ConcreteID, client.ContainerRemoveOptions{}); err != nil && !errdefs.IsNotFound(err) {
			return fmt.Errorf("remove Docker workload component %q: %w", component.Ref.Key.ComponentID, err)
		}
	}
	if err := os.RemoveAll(filepath.Join(e.config.DataRoot, "infrastructure-mount-targets", infrastructureContainerName(ref.Identity))); err != nil {
		return fmt.Errorf("remove Docker workload mount targets: %w", err)
	}
	return e.removeInfrastructureNetwork(ctx, ref.Identity)
}

func infrastructureWorkloadName(identity engine.GroupIdentity, workloadID string) string {
	return infrastructureContainerName(identity) + "-" + hashInfrastructureID(workloadID)[:10]
}

func workloadByID(workloads []engine.Workload, id string) engine.Workload {
	for _, workload := range workloads {
		if workload.ID == id {
			return workload
		}
	}
	return engine.Workload{}
}

func workloadEndpointMetadata(workload engine.Workload) map[string]string {
	result := make(map[string]string, len(workload.Endpoints))
	for _, endpoint := range workload.Endpoints {
		data, _ := json.Marshal(endpoint)
		result[endpoint.Name] = string(data)
	}
	return result
}

func workloadNamesFromGroup(identity engine.GroupIdentity, component string) string {
	return infrastructureWorkloadName(identity, component)
}
