package kubernetes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
)

const (
	infrastructureOwnerLabel = "orkest.infrastructure.owner"
	infrastructureGroupLabel = "orkest.infrastructure.group"
	infrastructureGeneration = "orkest.infrastructure.generation"
)

var _ engine.InfrastructureEngine = (*Engine)(nil)

func (e *Engine) ValidateStorageAccess(ctx context.Context, _ engine.StorageAccessRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return engine.Unsupported("Kubernetes managed workload storage access inspection")
}

func (e *Engine) ValidatePlan(ctx context.Context, plan engine.WorkloadPlan) error {
	if err := engine.ValidateWorkloadPlan(plan); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if plan.Group.InstallationID != "" && plan.Group.InstallationID != e.config.InstallationID {
		return engine.ErrIdentityMismatch
	}
	if len(plan.Workloads) != 1 || len(plan.Networks) != 0 || len(plan.Probes) != 0 {
		return engine.Unsupported("Kubernetes generic single-container workload shape")
	}
	workload := plan.Workloads[0]
	if workload.Mode != engine.ExecutionModeContainer {
		return engine.Unsupported("Kubernetes native workloads")
	}
	requirements := engine.Requirements{Mode: workload.Mode, ReadOnlyRoot: workload.Security.ReadOnlyRoot, RestartModes: workload.Restart != engine.RestartNever, CredentialBindings: len(workload.Credentials) != 0}
	for _, attachment := range plan.Storage {
		requirements.DurableWork = requirements.DurableWork || attachment.Durable
		requirements.DurableState = requirements.DurableState || attachment.Durable
		requirements.ReadOnlyMounts = requirements.ReadOnlyMounts || attachment.ReadOnly
	}
	for _, binding := range workload.Credentials {
		requirements.ReadOnlyMounts = requirements.ReadOnlyMounts || binding.ReadOnly
	}
	if err := engine.ValidateCapabilities(e.Capabilities(), requirements); err != nil {
		return err
	}
	if workload.WorkingPath != "" && !strings.HasPrefix(workload.WorkingPath, "/") {
		return fmt.Errorf("%w: container working directory must be absolute", engine.ErrInvalidSpec)
	}
	if len(workload.TemporaryMounts) != 0 || workload.Restart > engine.RestartOnFailure {
		return engine.Unsupported("Kubernetes temporary mounts or restart policy")
	}
	for _, item := range workload.Mounts {
		if item.SourceKind != engine.MountSourceStorage || !filepath.IsAbs(item.Source) {
			return engine.Unsupported("Kubernetes host and non-workspace mounts")
		}
		matched := false
		for _, attachment := range plan.Storage {
			if attachment.WorkloadID != workload.ID || attachment.StorageID == "" {
				continue
			}
			root, err := taskstorage.BindingPath(e.config.DataRoot, attachment.StorageID, attachment.Subpath)
			if err != nil {
				return fmt.Errorf("%w: invalid workspace mount storage identity", engine.ErrInvalidSpec)
			}
			relative, relErr := filepath.Rel(root, item.Source)
			if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				continue
			}
			resolved, resolveErr := filepath.EvalSymlinks(item.Source)
			if resolveErr != nil || resolved != item.Source {
				return fmt.Errorf("%w: workspace mount source has unstable ancestry", engine.ErrInvalidSpec)
			}
			matched = true
			break
		}
		if !matched {
			return engine.Unsupported("Kubernetes workspace mount is not backed by a declared storage attachment")
		}
	}
	if workload.Limits.ProcessCount > 0 {
		return engine.Unsupported("Kubernetes process-count limits")
	}
	for _, binding := range workload.Credentials {
		source, exists := e.config.CredentialSources[binding.Name]
		if !exists || source.Secret == "" || source.Key == "" || !filepath.IsAbs(binding.Target) || binding.Target == "/" {
			return engine.Unsupported("Kubernetes credential source is unavailable")
		}
	}
	for _, endpoint := range workload.Endpoints {
		if endpoint.ListenPort < 1 || endpoint.ListenHost != "" && endpoint.ListenHost != "0.0.0.0" {
			return fmt.Errorf("%w: Kubernetes endpoints require a port and wildcard bind", engine.ErrInvalidSpec)
		}
		if endpoint.Exposure == engine.ExposurePlatform {
			return engine.Unsupported("Kubernetes platform ingress routing")
		}
		if endpoint.Protocol != "tcp" && endpoint.Protocol != "udp" && endpoint.Protocol != "http" && endpoint.Protocol != "https" {
			return fmt.Errorf("%w: unsupported Kubernetes endpoint protocol", engine.ErrInvalidSpec)
		}
	}
	for name, reference := range workload.EnvironmentEndpointRefs {
		if reference.Workload != "" && reference.Workload != workload.ID {
			return engine.Unsupported("Kubernetes cross-workload environment endpoint references")
		}
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("%w: invalid endpoint environment name", engine.ErrInvalidSpec)
		}
	}
	for _, file := range plan.Files {
		if !utf8.Valid(file.Contents) {
			return engine.Unsupported("Kubernetes binary generated files")
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
	for _, binding := range request.Bindings {
		if binding.Name == "" {
			return nil, fmt.Errorf("%w: storage binding name is required", engine.ErrInvalidSpec)
		}
		identity := binding.StorageID
		subpath := binding.Subpath
		var path string
		providerPath := ""
		if identity == "" {
			if subpath != "" {
				return nil, fmt.Errorf("%w: storage subpath requires a stable storage identity", engine.ErrInvalidSpec)
			}
			identity = kubernetesStorageIdentity(request.Owner, request.Generation, binding.Name)
			subpath = filepath.ToSlash(filepath.Join("infrastructure-storage", identity))
			path = filepath.Join(e.config.DataRoot, filepath.FromSlash(subpath))
			providerPath = filepath.ToSlash(filepath.Join(e.config.DataSubpath, filepath.FromSlash(subpath)))
		} else {
			var pathErr error
			path, pathErr = taskstorage.BindingPath(e.config.DataRoot, identity, subpath)
			if pathErr != nil {
				return nil, fmt.Errorf("%w: invalid Kubernetes storage identity or subpath", engine.ErrInvalidSpec)
			}
			providerPath = filepath.ToSlash(filepath.Join(e.config.DataSubpath, "tasks", identity, filepath.FromSlash(subpath)))
		}
		if binding.Create {
			if err := os.MkdirAll(path, 0o700); err != nil {
				return nil, fmt.Errorf("create Kubernetes shared storage binding: %w", err)
			}
			if err := os.Chmod(path, 0o700); err != nil {
				return nil, fmt.Errorf("restrict Kubernetes shared storage binding: %w", err)
			}
		} else {
			info, err := os.Lstat(path)
			if err != nil {
				return nil, fmt.Errorf("inspect Kubernetes shared storage binding: %w", err)
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("%w: Kubernetes storage binding is not a regular directory", engine.ErrInvalidSpec)
			}
		}
		bindings = append(bindings, engine.StorageBinding{Name: binding.Name, Identity: identity, Subpath: subpath, HostPath: path, ProviderPath: providerPath, ProviderID: e.config.DataPVC})
	}
	return bindings, nil
}

func (e *Engine) StartPlan(ctx context.Context, plan engine.WorkloadPlan) (result engine.ResourceGroup, resultErr error) {
	if err := e.ValidatePlan(ctx, plan); err != nil {
		return engine.ResourceGroup{}, err
	}
	plan.Group.InstallationID = e.config.InstallationID
	manifests := e.manifests
	if err := manifests.Record(ctx, plan, "starting", nil); err != nil {
		return engine.ResourceGroup{}, err
	}
	defer func() {
		providerIDs := map[string]string{}
		for _, component := range result.Components {
			providerIDs[component.Ref.Key.ComponentID+"/"+component.Ref.Key.ResourceID] = component.Ref.ConcreteID
		}
		state := "uncertain"
		if resultErr == nil {
			state = "active"
		}
		if manifestErr := manifests.Record(context.WithoutCancel(ctx), plan, state, providerIDs); manifestErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("persist Kubernetes operation result: %w", manifestErr))
		}
	}()
	identity := plan.Group
	name := infrastructurePodName(identity)
	ref := engine.GroupRef{Identity: identity, Provider: e.processID(name, "")}
	prior, err := e.InspectGroup(ctx, engine.GroupRef{Identity: identity})
	if err != nil {
		return engine.ResourceGroup{}, fmt.Errorf("reconcile Kubernetes workload group before create: %w", err)
	}
	if prior.State != engine.GroupAbsent {
		return prior.Group, engine.ErrUncertain
	}
	workload := plan.Workloads[0]
	storageRequests := make([]engine.StorageBindingRequest, 0, len(plan.Storage))
	for _, attachment := range plan.Storage {
		storageRequests = append(storageRequests, engine.StorageBindingRequest{Name: attachment.Binding, StorageID: attachment.StorageID, Subpath: attachment.Subpath, Durable: attachment.Durable, Create: attachment.Create, PlatformIO: attachment.PlatformIO})
	}
	storageBindings, err := e.ResolveStorage(ctx, engine.StorageRequest{Owner: identity.Owner, Generation: identity.Generation, Bindings: storageRequests})
	if err != nil {
		return engine.ResourceGroup{}, err
	}
	storageByName := make(map[string]engine.StorageBinding, len(storageBindings))
	for _, binding := range storageBindings {
		storageByName[binding.Name] = binding
	}
	labels := infrastructureLabels(identity)
	annotations := map[string]string{infrastructureOwnerLabel: identity.Owner, infrastructureGeneration: identity.Generation, "orkest.infrastructure.component": workload.ID}
	credentialVolumes := make([]any, 0, len(workload.Credentials)+len(plan.Storage)+len(plan.Files))
	credentialMounts := make([]any, 0, len(workload.Credentials)+len(plan.Storage)+len(plan.Files))
	fileData := make(map[string]string, len(plan.Files))
	fileItems := make([]any, 0, len(plan.Files))
	fileMounts := make([]any, 0, len(plan.Files))
	for _, file := range plan.Files {
		key := "file-" + kubernetesStorageIdentity(file.Name, "generated", "key")[:12]
		fileData[key] = string(file.Contents)
		fileItems = append(fileItems, map[string]any{"key": key, "path": key, "mode": file.Mode})
		fileMounts = append(fileMounts, map[string]any{"name": "generated-files", "mountPath": file.Target, "subPath": key, "readOnly": true})
	}
	if len(fileData) > 0 {
		configMap := map[string]any{
			"apiVersion": "v1", "kind": "ConfigMap",
			"metadata": map[string]any{"name": name + "-files", "namespace": e.config.Namespace, "labels": labels, "annotations": annotations},
			"data":     fileData,
		}
		if _, err = e.ensureResource(ctx, e.resourcePath("configmaps", name+"-files"), configMap, labels); err != nil {
			return engine.ResourceGroup{Ref: ref}, errors.Join(fmt.Errorf("create Kubernetes generated files: %w", err), engine.ErrUncertain)
		}
		credentialVolumes = append(credentialVolumes, map[string]any{"name": "generated-files", "configMap": map[string]any{"name": name + "-files", "items": fileItems}})
		credentialMounts = append(credentialMounts, fileMounts...)
	}
	args := make([]string, 0, len(workload.Arguments))
	for _, argument := range workload.Arguments {
		if argument.Kind == engine.ArgumentLiteral {
			args = append(args, argument.Value)
			continue
		}
		endpoint, ok := endpointByName(workload.Endpoints, argument.Endpoint)
		if !ok {
			return engine.ResourceGroup{}, fmt.Errorf("%w: unknown endpoint %q", engine.ErrInvalidSpec, argument.Endpoint)
		}
		args = append(args, fmt.Sprintf("0.0.0.0:%d", endpoint.ListenPort))
	}
	env := make([]any, 0, len(workload.Environment)+len(workload.Credentials))
	environmentValues := make(map[string]string, len(workload.Environment))
	for key, value := range workload.Environment {
		if strings.TrimSpace(key) == "" || strings.Contains(key, "=") {
			return engine.ResourceGroup{}, fmt.Errorf("%w: invalid workload environment name", engine.ErrInvalidSpec)
		}
		environmentValues[key] = value
	}
	for key, reference := range workload.EnvironmentEndpointRefs {
		endpoint, ok := endpointByName(workload.Endpoints, reference.Endpoint)
		if !ok {
			return engine.ResourceGroup{}, fmt.Errorf("%w: unknown endpoint environment reference", engine.ErrInvalidSpec)
		}
		address := fmt.Sprintf("localhost:%d", endpoint.ListenPort)
		if endpoint.Protocol == "http" || endpoint.Protocol == "https" {
			address = endpoint.Protocol + "://" + address
		}
		environmentValues[key] = address
	}
	if len(environmentValues) > 0 {
		secretName := name + "-env"
		secret := map[string]any{
			"apiVersion": "v1", "kind": "Secret",
			"metadata": map[string]any{"name": secretName, "namespace": e.config.Namespace, "labels": labels, "annotations": annotations},
			"type":     "Opaque", "stringData": environmentValues,
		}
		if _, err = e.ensureResource(ctx, e.resourcePath("secrets", secretName), secret, labels); err != nil {
			return engine.ResourceGroup{Ref: ref}, errors.Join(fmt.Errorf("create Kubernetes workload environment Secret: %w", err), engine.ErrUncertain)
		}
		for key := range environmentValues {
			env = append(env, map[string]any{"name": key, "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": secretName, "key": key}}})
		}
	}
	for _, binding := range workload.Credentials {
		source := e.config.CredentialSources[binding.Name]
		volumeName := "credential-" + binding.Name
		fileName := filepath.Base(binding.Target)
		credentialVolumes = append(credentialVolumes, map[string]any{"name": volumeName, "secret": map[string]any{"secretName": source.Secret, "items": []any{map[string]any{"key": source.Key, "path": fileName}}}})
		credentialMounts = append(credentialMounts, map[string]any{"name": volumeName, "mountPath": binding.Target, "subPath": fileName, "readOnly": binding.ReadOnly})
		if binding.EnvironmentName != "" {
			env = append(env, map[string]any{"name": binding.EnvironmentName, "value": binding.Target})
		}
	}
	for _, attachment := range plan.Storage {
		binding, exists := storageByName[attachment.Binding]
		if !exists || binding.ProviderID != e.config.DataPVC {
			return engine.ResourceGroup{}, fmt.Errorf("%w: storage binding %q was not resolved against the configured PVC", engine.ErrInvalidSpec, attachment.Binding)
		}
		volumeName := "storage-" + kubernetesStorageIdentity(identity.Owner, identity.Generation, attachment.WorkloadID+"-"+attachment.Binding)[:12]
		subpath := binding.ProviderPath
		credentialVolumes = append(credentialVolumes, map[string]any{"name": volumeName, "persistentVolumeClaim": map[string]any{"claimName": binding.ProviderID, "readOnly": attachment.ReadOnly}})
		credentialMounts = append(credentialMounts, map[string]any{"name": volumeName, "mountPath": attachment.Target, "subPath": subpath, "readOnly": attachment.ReadOnly})
	}
	for _, item := range workload.Mounts {
		var binding engine.StorageBinding
		found := false
		for _, attachment := range plan.Storage {
			if attachment.WorkloadID != workload.ID {
				continue
			}
			candidate, exists := storageByName[attachment.Binding]
			if !exists {
				continue
			}
			relative, relErr := filepath.Rel(candidate.HostPath, item.Source)
			if relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				binding, found = candidate, true
				break
			}
		}
		if !found || binding.ProviderID != e.config.DataPVC {
			return engine.ResourceGroup{Ref: ref}, engine.Unsupported("Kubernetes workspace mount has no PVC storage binding")
		}
		relative, relErr := filepath.Rel(binding.HostPath, item.Source)
		if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return engine.ResourceGroup{Ref: ref}, fmt.Errorf("%w: Kubernetes workspace mount escapes its storage root", engine.ErrInvalidSpec)
		}
		subpath := filepath.ToSlash(filepath.Join(filepath.FromSlash(binding.ProviderPath), relative))
		volumeName := "storage-" + kubernetesStorageIdentity(identity.Owner, identity.Generation, item.Target)[:12]
		credentialVolumes = append(credentialVolumes, map[string]any{"name": volumeName, "persistentVolumeClaim": map[string]any{"claimName": binding.ProviderID, "readOnly": item.ReadOnly}})
		credentialMounts = append(credentialMounts, map[string]any{"name": volumeName, "mountPath": item.Target, "subPath": subpath, "readOnly": item.ReadOnly})
	}
	runAsUID, runAsGID := e.config.UID, e.config.GID
	if workload.Security.RunAsUser > 0 {
		runAsUID = int64(workload.Security.RunAsUser)
	}
	if workload.Security.RunAsGroup > 0 {
		runAsGID = int64(workload.Security.RunAsGroup)
	}
	containerSecurity := map[string]any{
		"runAsNonRoot": true, "runAsUser": runAsUID, "runAsGroup": runAsGID,
		"readOnlyRootFilesystem":   workload.Security.ReadOnlyRoot,
		"allowPrivilegeEscalation": false,
	}
	if workload.Security.DropAllCapabilities {
		containerSecurity["capabilities"] = map[string]any{"drop": []string{"ALL"}}
	}
	resources := map[string]any{"requests": map[string]string{"cpu": e.config.CPURequest, "memory": e.config.MemoryRequest}, "limits": map[string]string{"cpu": e.config.CPULimit, "memory": e.config.MemoryLimit}}
	if workload.Limits.NanoCPUs > 0 {
		resources["limits"].(map[string]string)["cpu"] = fmt.Sprintf("%dn", workload.Limits.NanoCPUs)
	}
	if workload.Limits.MemoryBytes > 0 {
		resources["limits"].(map[string]string)["memory"] = fmt.Sprintf("%d", workload.Limits.MemoryBytes)
	}
	containerSpec := map[string]any{
		"name": "workload", "image": workload.Artifact.ContainerImage,
		"command": workload.Artifact.ContainerEntrypoint, "args": args,
		"env": env, "workingDir": workload.WorkingPath,
		"securityContext": containerSecurity,
		"resources":       resources,
	}
	containerPorts := make([]any, 0, len(workload.Endpoints))
	servicePorts := make([]any, 0, len(workload.Endpoints))
	for _, endpoint := range workload.Endpoints {
		protocol := strings.ToUpper(endpoint.Protocol)
		if protocol == "HTTP" || protocol == "HTTPS" {
			protocol = "TCP"
		}
		containerPorts = append(containerPorts, map[string]any{"name": endpoint.Name, "containerPort": endpoint.ListenPort, "protocol": protocol})
		servicePorts = append(servicePorts, map[string]any{"name": endpoint.Name, "port": endpoint.ListenPort, "targetPort": endpoint.Name, "protocol": protocol})
		if endpoint.ReadinessPath != "" {
			containerSpec["readinessProbe"] = map[string]any{"httpGet": map[string]any{"path": endpoint.ReadinessPath, "port": endpoint.Name}, "periodSeconds": 2, "failureThreshold": 90}
		}
	}
	containerSpec["ports"] = containerPorts
	containerSpec["volumeMounts"] = credentialMounts
	if len(servicePorts) != 0 {
		service := map[string]any{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]any{"name": name, "namespace": e.config.Namespace, "labels": labels, "annotations": annotations},
			"spec":     map[string]any{"type": "ClusterIP", "selector": map[string]string{infrastructureGroupLabel: infrastructureGroupHash(identity)}, "ports": servicePorts},
		}
		if _, err = e.ensureResource(ctx, e.resourcePath("services", name), service, labels); err != nil {
			return engine.ResourceGroup{Ref: ref}, errors.Join(fmt.Errorf("create Kubernetes infrastructure endpoint service: %w", err), engine.ErrUncertain)
		}
	}
	resource := map[string]any{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]any{"name": name, "namespace": e.config.Namespace, "labels": labels, "annotations": annotations},
		"spec":     map[string]any{"restartPolicy": map[engine.RestartPolicy]string{engine.RestartNever: "Never", engine.RestartOnFailure: "OnFailure"}[workload.Restart], "automountServiceAccountToken": false, "terminationGracePeriodSeconds": e.config.TerminationGraceSeconds, "securityContext": map[string]any{"runAsNonRoot": true, "runAsUser": runAsUID, "runAsGroup": runAsGID}, "containers": []any{containerSpec}, "volumes": credentialVolumes},
	}
	created, err := e.ensureResource(ctx, e.resourcePath("pods", name), resource, labels)
	if err != nil {
		return engine.ResourceGroup{}, fmt.Errorf("create Kubernetes infrastructure workload: %w", err)
	}
	uid := nestedStringValue(created, "metadata", "uid")
	ref.Provider = e.processID(name, uid)
	for {
		current, status, getErr := e.client.Do(ctx, http.MethodGet, e.resourcePath("pods", name), nil)
		if getErr != nil || status >= 300 {
			return engine.ResourceGroup{Ref: ref}, errors.Join(fmt.Errorf("inspect Kubernetes workload startup: HTTP %d: %w", status, getErr), engine.ErrUncertain)
		}
		phase := nestedStringValue(current, "status", "phase")
		if phase == "Running" {
			return e.infrastructurePodGroup(ctx, identity, name, uid, workload.ID, engine.ResourceRunning)
		}
		if phase == "Failed" || phase == "Succeeded" {
			group, groupErr := e.infrastructurePodGroup(ctx, identity, name, uid, workload.ID, engine.ResourceStopped)
			return group, errors.Join(errors.New("workload stopped before becoming ready"), groupErr)
		}
		select {
		case <-ctx.Done():
			return engine.ResourceGroup{Ref: ref}, errors.Join(ctx.Err(), engine.ErrUncertain)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (e *Engine) InspectGroup(ctx context.Context, ref engine.GroupRef) (engine.GroupObservation, error) {
	if ref.Identity.InstallationID != e.config.InstallationID {
		return engine.GroupObservation{}, engine.ErrIdentityMismatch
	}
	name, uid := infrastructurePodName(ref.Identity), ""
	if ref.Provider != "" {
		var err error
		name, uid, err = e.parseID(ref.Provider)
		if err != nil {
			return engine.GroupObservation{}, engine.ErrIdentityMismatch
		}
	}
	value, status, err := e.client.Do(ctx, http.MethodGet, e.resourcePath("pods", name), nil)
	if err != nil {
		return engine.GroupObservation{}, err
	}
	if status == http.StatusNotFound {
		group, groupErr := e.infrastructurePodGroup(ctx, ref.Identity, name, uid, "", engine.ResourceStopped)
		if groupErr != nil {
			return engine.GroupObservation{}, groupErr
		}
		if len(group.Components) == 0 {
			return engine.GroupObservation{State: engine.GroupAbsent}, nil
		}
		return engine.GroupObservation{State: engine.GroupStopped, Group: group}, nil
	}
	if status >= 300 {
		return engine.GroupObservation{}, fmt.Errorf("inspect Kubernetes infrastructure group: HTTP %d", status)
	}
	labels, _ := nestedMap(value, "metadata", "labels")
	annotations, _ := nestedMap(value, "metadata", "annotations")
	if labels[infrastructureGroupLabel] != infrastructureGroupHash(ref.Identity) || labels["orkest.installation"] != e.config.InstallationID || annotations[infrastructureOwnerLabel] != ref.Identity.Owner || annotations[infrastructureGeneration] != ref.Identity.Generation {
		return engine.GroupObservation{}, engine.ErrIdentityMismatch
	}
	observedUID := nestedStringValue(value, "metadata", "uid")
	if uid != "" && observedUID != uid {
		return engine.GroupObservation{}, engine.ErrIdentityMismatch
	}
	phase := nestedStringValue(value, "status", "phase")
	state, resourceState := engine.GroupUncertain, engine.ResourceUnknown
	switch phase {
	case "Pending", "Running":
		state, resourceState = engine.GroupActive, engine.ResourceRunning
	case "Succeeded", "Failed":
		state, resourceState = engine.GroupStopped, engine.ResourceStopped
	}
	group, groupErr := e.infrastructurePodGroup(ctx, ref.Identity, name, observedUID, nestedStringValue(annotations, "orkest.infrastructure.component"), resourceState)
	if groupErr != nil {
		return engine.GroupObservation{}, groupErr
	}
	return engine.GroupObservation{State: state, Group: group}, nil
}

func (e *Engine) ListGroups(ctx context.Context, selector engine.Selector) ([]engine.ResourceGroup, error) {
	if selector.InstallationID != "" && selector.InstallationID != e.config.InstallationID {
		return nil, engine.ErrIdentityMismatch
	}
	list, status, err := e.client.Do(ctx, http.MethodGet, e.resourcePath("pods", ""), nil)
	if err != nil || status >= 300 {
		return nil, errors.Join(fmt.Errorf("list Kubernetes infrastructure groups: HTTP %d: %w", status, err), err)
	}
	items, _ := list["items"].([]any)
	services, serviceStatus, serviceErr := e.client.Do(ctx, http.MethodGet, e.resourcePath("services", ""), nil)
	if serviceErr != nil || serviceStatus >= 300 && serviceStatus != http.StatusNotFound {
		return nil, errors.Join(fmt.Errorf("list Kubernetes infrastructure services: HTTP %d: %w", serviceStatus, serviceErr), serviceErr)
	}
	if serviceStatus != http.StatusNotFound {
		serviceItems, _ := services["items"].([]any)
		items = append(items, serviceItems...)
	}
	configMaps, configMapStatus, configMapErr := e.client.Do(ctx, http.MethodGet, e.resourcePath("configmaps", ""), nil)
	if configMapErr != nil || configMapStatus >= 300 && configMapStatus != http.StatusNotFound {
		return nil, errors.Join(fmt.Errorf("list Kubernetes infrastructure ConfigMaps: HTTP %d: %w", configMapStatus, configMapErr), configMapErr)
	}
	if configMapStatus != http.StatusNotFound {
		configMapItems, _ := configMaps["items"].([]any)
		items = append(items, configMapItems...)
	}
	secrets, secretStatus, secretErr := e.client.Do(ctx, http.MethodGet, e.resourcePath("secrets", ""), nil)
	if secretErr != nil || secretStatus >= 300 && secretStatus != http.StatusNotFound {
		return nil, errors.Join(fmt.Errorf("list Kubernetes infrastructure Secrets: HTTP %d: %w", secretStatus, secretErr), secretErr)
	}
	if secretStatus != http.StatusNotFound {
		secretItems, _ := secrets["items"].([]any)
		items = append(items, secretItems...)
	}
	groups := make([]engine.ResourceGroup, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		value, ok := item.(map[string]any)
		if !ok {
			continue
		}
		labels, _ := nestedMap(value, "metadata", "labels")
		annotations, _ := nestedMap(value, "metadata", "annotations")
		if labels[infrastructureGroupLabel] == "" || labels["orkest.installation"] != e.config.InstallationID || selector.GroupOwner != "" && annotations[infrastructureOwnerLabel] != selector.GroupOwner {
			continue
		}
		identity := engine.GroupIdentity{InstallationID: e.config.InstallationID, Owner: nestedStringValue(annotations, infrastructureOwnerLabel), Generation: nestedStringValue(annotations, infrastructureGeneration)}
		if _, exists := seen[identity.Owner+"\x00"+identity.Generation]; exists {
			continue
		}
		seen[identity.Owner+"\x00"+identity.Generation] = struct{}{}
		observation, inspectErr := e.InspectGroup(ctx, engine.GroupRef{Identity: identity, Provider: e.processID(infrastructurePodName(identity), "")})
		if inspectErr != nil {
			return nil, inspectErr
		}
		groups = append(groups, observation.Group)
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
	name, _, err := e.parseID(observation.Group.Ref.Provider)
	if err != nil {
		return err
	}
	_, status, err := e.client.Do(ctx, http.MethodDelete, e.resourcePath("pods", name), map[string]any{"apiVersion": "v1", "kind": "DeleteOptions"})
	if err != nil || status >= 300 && status != http.StatusNotFound {
		return fmt.Errorf("stop Kubernetes infrastructure workload: HTTP %d: %w", status, err)
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		observation, inspectErr := e.InspectGroup(ctx, ref)
		if inspectErr != nil {
			return inspectErr
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
	if observation.State == engine.GroupActive || observation.State == engine.GroupUncertain {
		return errors.New("refusing to clean active or uncertain Kubernetes infrastructure group")
	}
	if err = e.deleteOwned(ctx, "pods", infrastructurePodName(ref.Identity)); err != nil {
		return err
	}
	if err = e.deleteOwned(ctx, "services", infrastructurePodName(ref.Identity)); err != nil {
		return err
	}
	if err = e.deleteOwned(ctx, "configmaps", infrastructurePodName(ref.Identity)+"-files"); err != nil {
		return err
	}
	return e.deleteOwned(ctx, "secrets", infrastructurePodName(ref.Identity)+"-env")
}

func infrastructureGroupHash(identity engine.GroupIdentity) string {
	hash := sha256.Sum256([]byte(identity.InstallationID + "\x00" + identity.Owner + "\x00" + identity.Generation))
	return hex.EncodeToString(hash[:])
}

func infrastructurePodName(identity engine.GroupIdentity) string {
	return "orkest-workload-" + infrastructureGroupHash(identity)[:32]
}

func infrastructureLabels(identity engine.GroupIdentity) map[string]string {
	return map[string]string{
		"orkest.installation":           identity.InstallationID,
		infrastructureGroupLabel:        infrastructureGroupHash(identity),
		infrastructureOwnerLabel:        infrastructureGroupHash(engine.GroupIdentity{Owner: identity.Owner}),
		infrastructureGeneration:        infrastructureGroupHash(engine.GroupIdentity{Generation: identity.Generation}),
		"orkest.infrastructure.version": "1",
	}
}

func (e *Engine) infrastructurePodGroup(ctx context.Context, identity engine.GroupIdentity, name, uid, component string, state engine.ResourceState) (engine.ResourceGroup, error) {
	group := engine.ResourceGroup{Ref: engine.GroupRef{Identity: identity, Provider: e.processID(name, uid)}, Endpoints: make(map[string]string)}
	if component != "" {
		group.Components = append(group.Components, engine.Resource{Ref: engine.ResourceRef{Key: engine.ResourceKey{InstallationID: identity.InstallationID, ResourceID: name, ComponentID: component}, ConcreteID: uid}, State: state})
	}
	service, status, err := e.client.Do(ctx, http.MethodGet, e.resourcePath("services", name), nil)
	if err != nil {
		return group, err
	}
	if status >= 300 && status != http.StatusNotFound {
		return group, fmt.Errorf("inspect Kubernetes infrastructure endpoint service: HTTP %d", status)
	}
	if status != http.StatusNotFound {
		if err = verifyLabels(service, infrastructureLabels(identity), name); err != nil {
			return group, err
		}
		serviceUID := nestedStringValue(service, "metadata", "uid")
		group.Components = append(group.Components, engine.Resource{Ref: engine.ResourceRef{Key: engine.ResourceKey{InstallationID: identity.InstallationID, ResourceID: name, ComponentID: "service"}, ConcreteID: serviceUID}, State: engine.ResourceRunning})
		ports, _ := nested(service, "spec", "ports").([]any)
		for _, raw := range ports {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			portName, _ := item["name"].(string)
			port, _ := item["port"].(float64)
			protocol, _ := item["protocol"].(string)
			address := fmt.Sprintf("%s.%s.svc:%d", name, e.config.Namespace, int(port))
			if strings.EqualFold(protocol, "TCP") {
				address = "http://" + address
			}
			group.Endpoints[portName] = address
		}
	}
	configMap, configMapStatus, configMapErr := e.client.Do(ctx, http.MethodGet, e.resourcePath("configmaps", name+"-files"), nil)
	if configMapErr != nil {
		return group, configMapErr
	}
	if configMapStatus >= 300 && configMapStatus != http.StatusNotFound {
		return group, fmt.Errorf("inspect Kubernetes generated files: HTTP %d", configMapStatus)
	}
	if configMapStatus != http.StatusNotFound {
		if err = verifyLabels(configMap, infrastructureLabels(identity), name+"-files"); err != nil {
			return group, err
		}
		group.Components = append(group.Components, engine.Resource{Ref: engine.ResourceRef{Key: engine.ResourceKey{InstallationID: identity.InstallationID, ResourceID: name, ComponentID: "files"}, ConcreteID: nestedStringValue(configMap, "metadata", "uid")}, State: engine.ResourceRunning})
	}
	secret, secretStatus, secretErr := e.client.Do(ctx, http.MethodGet, e.resourcePath("secrets", name+"-env"), nil)
	if secretErr != nil {
		return group, secretErr
	}
	if secretStatus >= 300 && secretStatus != http.StatusNotFound {
		return group, fmt.Errorf("inspect Kubernetes workload environment Secret: HTTP %d", secretStatus)
	}
	if secretStatus != http.StatusNotFound {
		if err = verifyLabels(secret, infrastructureLabels(identity), name+"-env"); err != nil {
			return group, err
		}
		group.Components = append(group.Components, engine.Resource{Ref: engine.ResourceRef{Key: engine.ResourceKey{InstallationID: identity.InstallationID, ResourceID: name, ComponentID: "environment"}, ConcreteID: nestedStringValue(secret, "metadata", "uid")}, State: engine.ResourceRunning})
	}
	return group, nil
}

func kubernetesStorageIdentity(owner, generation, binding string) string {
	hash := sha256.Sum256([]byte(owner + "\x00" + generation + "\x00" + binding))
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
