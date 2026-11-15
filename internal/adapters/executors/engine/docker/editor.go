package docker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	client "github.com/moby/moby/client"
)

const (
	dockerEditorLabel   = "io.orkest.editor"
	editorRelayTokenEnv = "ORKEST_EDITOR_RELAY_TOKEN"
)

func (e *ExecutorEngine) startEditor(ctx context.Context, spec engine.LaunchSpec) (engine.Resource, error) {
	e.editorMu.Lock()
	defer e.editorMu.Unlock()
	resource := engine.Resource{Ref: engine.ResourceRef{Key: spec.Key}}
	if e.editorClient == nil {
		return resource, engine.Unsupported("Docker editor lifecycle API")
	}
	if spec.Key.Scope.OrganizationID == "" || spec.Key.Scope.WorkspaceID == "" || spec.Key.Scope.TaskID == "" || spec.Metadata.OwnerID == "" || len(spec.Key.ExecutionID) != 64 {
		return resource, fmt.Errorf("%w: editor scope, owner, and generation are required", engine.ErrInvalidSpec)
	}
	if spec.Restart != engine.RestartNever || spec.Artifact.ContainerImage == "" {
		return resource, fmt.Errorf("%w: editor image and no-restart policy are required", engine.ErrInvalidSpec)
	}
	if !spec.Security.ReadOnlyRoot || spec.Security.RunAsUser == 0 || spec.Security.RunAsGroup == 0 || !spec.Security.DropAllCapabilities || !spec.Security.NoPrivilegeEscalation || spec.Limits.MemoryBytes <= 0 || spec.Limits.NanoCPUs <= 0 || spec.Limits.ProcessCount <= 0 {
		return resource, fmt.Errorf("%w: editor isolation and resource limits are required", engine.ErrInvalidSpec)
	}
	if !spec.Sharing.ProtectedAncestry || len(spec.Environment) != 0 || len(spec.CredentialBindings) != 0 || len(spec.HostEnvironment.AllowedNames) != 0 || spec.HostEnvironment.InheritAllowlisted {
		return resource, fmt.Errorf("%w: editor must use protected storage without host or credential environment", engine.ErrInvalidSpec)
	}
	if len(spec.Endpoints) != 1 || spec.Endpoints[0].Protocol != "http" || spec.Endpoints[0].ListenPort != 8080 || spec.Endpoints[0].Ingress != engine.IngressGatewayOnly || len(spec.TemporaryMounts) != 2 {
		return resource, fmt.Errorf("%w: editor gateway endpoint and temporary mounts are required", engine.ErrInvalidSpec)
	}
	if spec.Key.InstallationID != "" && spec.Key.InstallationID != e.installation {
		return resource, engine.ErrIdentityMismatch
	}
	if err := e.Validate(ctx, spec.Requirements); err != nil {
		return resource, err
	}
	if err := e.requireProtectedWorkers(ctx, spec.Key.Scope, spec.Workspace.WorkPath); err != nil {
		return resource, err
	}
	workspace, err := e.ResolveWorkspace(ctx, engine.WorkspaceRequest{Scope: spec.Key.Scope, Intent: engine.WorkspaceInspect, Required: engine.WorkspaceGuarantees{DurableWork: true}})
	if err != nil {
		return resource, err
	}
	resolvedWork, err := filepath.EvalSymlinks(workspace.WorkPath)
	if err != nil || !workspace.Exists || resolvedWork != spec.Workspace.WorkPath {
		return resource, fmt.Errorf("%w: Docker editor workspace changed before launch", engine.ErrIdentityMismatch)
	}
	name := editorName(e.installation, spec.Key.Scope)
	resource.Ref.Key.InstallationID = e.installation
	resource.Ref.ConcreteID = name
	networkName := editorNetworkName(name, spec.Key.ExecutionID)
	manifestDirectory := filepath.Join(e.config.DataRoot, "editor-workspaces", name, spec.Key.ExecutionID)
	if err = os.MkdirAll(manifestDirectory, 0o700); err != nil {
		return resource, fmt.Errorf("create editor manifest directory: %w", err)
	}
	retained := false
	defer func() {
		if !retained {
			_ = os.RemoveAll(manifestDirectory)
		}
	}()
	var manifest *engine.GeneratedFile
	for i := range spec.GeneratedFiles {
		if spec.GeneratedFiles[i].Name == "task.code-workspace" && spec.GeneratedFiles[i].Target == "/workspace/task.code-workspace" {
			manifest = &spec.GeneratedFiles[i]
			break
		}
	}
	if manifest == nil || manifest.Mode != 0o444 {
		return resource, fmt.Errorf("%w: read-only editor workspace manifest is required", engine.ErrInvalidSpec)
	}
	manifestPath := filepath.Join(manifestDirectory, "task.code-workspace")
	if err = os.WriteFile(manifestPath, manifest.Contents, 0o444); err != nil {
		return resource, fmt.Errorf("write editor workspace manifest: %w", err)
	}
	mounts := []mount.Mount{
		{Type: mount.TypeBind, Source: manifestPath, Target: manifest.Target, ReadOnly: true},
	}
	for _, item := range spec.Mounts {
		if item.SourceKind != engine.MountSourceWorkspace || item.Source != resolvedWork || item.Target != "/task" {
			return resource, engine.Unsupported("Docker editor mount source or target")
		}
		mounts = append(mounts, mount.Mount{Type: mount.TypeBind, Source: resolvedWork, Target: item.Target, ReadOnly: item.ReadOnly})
	}
	if len(mounts) != 2 {
		return resource, fmt.Errorf("%w: one task workspace mount is required", engine.ErrInvalidSpec)
	}
	if spec.Metadata.Mode == "ro" && !mounts[1].ReadOnly || spec.Metadata.Mode == "rw" && mounts[1].ReadOnly || (spec.Metadata.Mode != "ro" && spec.Metadata.Mode != "rw") {
		return resource, fmt.Errorf("%w: editor mode and task mount access disagree", engine.ErrInvalidSpec)
	}
	labels := editorLabels(e.installation, spec.Key.Scope, spec.Key.ExecutionID, spec.Metadata)
	if err = e.createEditorNetwork(ctx, networkName, labels); err != nil {
		return resource, err
	}
	containerCreated := false
	defer func() {
		if !containerCreated {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = e.removeEditorNetwork(cleanupCtx, networkName, spec.Key, spec.Metadata)
		}
	}()
	created, err := e.editorClient.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name:       name,
		Config:     &container.Config{Image: spec.Artifact.ContainerImage, User: fmt.Sprintf("%d:%d", spec.Security.RunAsUser, spec.Security.RunAsGroup), Env: []string{"HOME=/home/editor", "EDITOR_MODE=" + spec.Metadata.Mode}, Labels: labels, ExposedPorts: network.PortSet{network.MustParsePort("8080/tcp"): {}}},
		HostConfig: editorHostConfig(spec, mounts, networkName),
	})
	if err != nil {
		current, inspectErr := e.editorClient.ContainerInspect(ctx, name, client.ContainerInspectOptions{})
		if inspectErr == nil {
			observed, _, decodeErr := e.decodeEditor(current.Container, spec.Key)
			if decodeErr != nil {
				return resource, errors.Join(engine.ErrIdentityMismatch, err)
			}
			resource.Ref.ConcreteID = observed.Ref.ConcreteID
			containerCreated = true
			retained = true
			return resource, errors.Join(engine.ErrUncertain, fmt.Errorf("create Docker editor: %w", err))
		}
		if !errdefs.IsNotFound(inspectErr) {
			resource.Ref.ConcreteID = name
			containerCreated = true
			retained = true
			return resource, errors.Join(engine.ErrUncertain, fmt.Errorf("create Docker editor: %w", err), fmt.Errorf("inspect uncertain Docker editor creation: %w", inspectErr))
		}
		return resource, fmt.Errorf("create Docker editor: %w", err)
	}
	if created.ID == "" {
		resource.Ref.ConcreteID = name
		containerCreated = true
		retained = true
		return resource, engine.ErrUncertain
	}
	resource.Ref.ConcreteID = created.ID
	containerCreated = true
	retained = true
	if _, err = e.editorClient.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return resource, fmt.Errorf("start Docker editor: %w", err)
	}
	resource.Endpoint = "http://" + name + ":8080"
	if e.config.NetworkName == "" {
		resource.Access, err = e.startEditorRelay(ctx, spec, networkName, labels)
		if err != nil {
			return resource, fmt.Errorf("start Docker editor relay: %w", err)
		}
		relay, relayErr := e.editorClient.ContainerInspect(ctx, networkName+"-relay", client.ContainerInspectOptions{})
		if relayErr != nil {
			return resource, relayErr
		}
		resource.Endpoint, err = publishedEndpoint(relay.Container, 8081)
		if err != nil {
			return resource, err
		}
	}
	resource.State = engine.ResourceRunning
	readyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	probe := &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for {
		request, requestErr := http.NewRequestWithContext(readyCtx, http.MethodGet, resource.Endpoint+"/healthz", nil)
		if requestErr == nil {
			if resource.Access != "" {
				request.Header.Set("X-Orkest-Editor-Relay", resource.Access)
			}
			response, doErr := probe.Do(request)
			if doErr == nil {
				_ = response.Body.Close()
				if response.StatusCode == http.StatusOK {
					return resource, nil
				}
			}
		}
		select {
		case <-readyCtx.Done():
			return resource, fmt.Errorf("docker editor readiness: %w", readyCtx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func editorHostConfig(spec engine.LaunchSpec, mounts []mount.Mount, networkName string) *container.HostConfig {
	tmpfs := make(map[string]string, len(spec.TemporaryMounts))
	for _, item := range spec.TemporaryMounts {
		tmpfs[item.Target] = tmpfsOptions(item)
	}
	limits := container.Resources{Memory: spec.Limits.MemoryBytes, NanoCPUs: spec.Limits.NanoCPUs}
	if spec.Limits.ProcessCount > 0 {
		pids := spec.Limits.ProcessCount
		limits.PidsLimit = &pids
	}
	capDrop := []string(nil)
	if spec.Security.DropAllCapabilities {
		capDrop = []string{"ALL"}
	}
	security := []string(nil)
	if spec.Security.NoPrivilegeEscalation {
		security = []string{"no-new-privileges:true"}
	}
	return &container.HostConfig{Mounts: mounts, NetworkMode: container.NetworkMode(networkName), ReadonlyRootfs: spec.Security.ReadOnlyRoot, CapDrop: capDrop, SecurityOpt: security, Tmpfs: tmpfs, Resources: limits, RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyDisabled}}
}

func (e *ExecutorEngine) createEditorNetwork(ctx context.Context, name string, labels map[string]string) error {
	if e.editorClient == nil {
		return engine.Unsupported("Docker editor lifecycle API")
	}
	gatewayID := ""
	if e.config.NetworkName != "" {
		hostname, err := os.Hostname()
		if err != nil {
			return err
		}
		gateway, inspectErr := e.editorClient.ContainerInspect(ctx, hostname, client.ContainerInspectOptions{})
		if inspectErr != nil {
			return fmt.Errorf("inspect Docker editor gateway: %w", inspectErr)
		}
		if gateway.Container.NetworkSettings == nil || gateway.Container.NetworkSettings.Networks[e.config.NetworkName] == nil {
			return errors.New("docker editor gateway is not attached to the configured platform network")
		}
		gatewayID = gateway.Container.ID
	}
	created, err := e.editorClient.NetworkCreate(ctx, name, client.NetworkCreateOptions{Driver: "bridge", Internal: true, Labels: labels})
	if err != nil {
		return fmt.Errorf("create private Docker editor network: %w", err)
	}
	if gatewayID != "" {
		if _, err = e.editorClient.NetworkConnect(ctx, created.ID, client.NetworkConnectOptions{Container: gatewayID}); err != nil {
			_, _ = e.editorClient.NetworkRemove(context.WithoutCancel(ctx), created.ID, client.NetworkRemoveOptions{})
			return fmt.Errorf("attach gateway to private Docker editor network: %w", err)
		}
	}
	return nil
}

func (e *ExecutorEngine) requireProtectedWorkers(ctx context.Context, scope engine.Scope, work string) error {
	filters := client.Filters{}.Add("label", "orkest.managed=agentctl")
	for label, value := range map[string]string{"organization": scope.OrganizationID, "workspace": scope.WorkspaceID, "task": scope.TaskID} {
		hash := sha256.Sum256([]byte(value))
		filters = filters.Add("label", "orkest."+label+"_id_hash="+hex.EncodeToString(hash[:]))
	}
	listed, err := e.editorClient.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: filters})
	if err != nil {
		return fmt.Errorf("list Docker workers before editor admission: %w", err)
	}
	for _, item := range listed.Items {
		inspected, inspectErr := e.editorClient.ContainerInspect(ctx, item.ID, client.ContainerInspectOptions{})
		if errdefs.IsNotFound(inspectErr) {
			continue
		}
		if inspectErr != nil {
			return inspectErr
		}
		if err := validateProtectedWorker(inspected.Container, work); err != nil {
			return fmt.Errorf("%w: existing Docker worker mounts are unsafe: %v", engine.ErrUnsupported, err)
		}
	}
	resolved, err := filepath.EvalSymlinks(work)
	if err != nil || resolved != work {
		return engine.Unsupported("Docker task workspace has unstable ancestry")
	}
	return nil
}

func validateProtectedWorker(worker container.InspectResponse, work string) error {
	if worker.Config == nil || worker.Config.Labels[checkoutMountLayoutLabel] != checkoutMountLayout || worker.HostConfig == nil || worker.HostConfig.Privileged {
		return errors.New("worker does not use protected checkout mounts")
	}
	for _, capability := range worker.HostConfig.CapAdd {
		capability = strings.TrimPrefix(strings.ToUpper(capability), "CAP_")
		if capability == "SYS_ADMIN" || capability == "ALL" {
			return errors.New("worker can change checkout mounts")
		}
	}
	return validateProtectedWorkerMounts(worker, work)
}

func validateProtectedWorkerMounts(worker container.InspectResponse, work string) error {
	work, err := filepath.EvalSymlinks(work)
	if err != nil {
		return err
	}
	repositories := filepath.Join(work, "repositories")
	protected := false
	for _, item := range worker.Mounts {
		source, resolveErr := filepath.EvalSymlinks(item.Source)
		if resolveErr != nil {
			return fmt.Errorf("resolve worker mount source: %w", resolveErr)
		}
		if item.Destination == workMountPath+"/repositories" && source == repositories && !item.RW {
			protected = true
		}
		if !item.RW || item.Destination == workMountPath && source == work {
			continue
		}
		if pathWithin(work, source) {
			if !pathWithin(repositories, source) || source == repositories {
				return errors.New("worker has a writable task workspace alias")
			}
			relative, relErr := filepath.Rel(work, source)
			if relErr != nil || item.Destination != workMountPath+"/"+filepath.ToSlash(relative) {
				return errors.New("worker has a writable checkout ancestry alias")
			}
			parts := strings.Split(filepath.ToSlash(relative), "/")
			if len(parts) != 3 || parts[0] != "repositories" {
				return errors.New("worker has a writable checkout ancestry alias")
			}
		}
		if pathWithin(source, work) {
			return errors.New("worker has a writable task workspace ancestry alias")
		}
	}
	if !protected {
		return errors.New("worker checkout parent is writable")
	}
	return nil
}

func pathWithin(root, value string) bool {
	relative, err := filepath.Rel(root, value)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func editorName(installation string, scope engine.Scope) string {
	data, _ := json.Marshal([]string{installation, scope.OrganizationID, scope.WorkspaceID, scope.TaskID})
	hash := sha256.Sum256(data)
	return "orkest-editor-" + hex.EncodeToString(hash[:])[:40]
}

func editorNetworkName(name, generation string) string {
	hash := sha256.Sum256([]byte(name + ":" + generation))
	return "orkest-editor-net-" + hex.EncodeToString(hash[:])[:48]
}

func editorLabels(installation string, scope engine.Scope, generation string, metadata engine.OwnershipMetadata) map[string]string {
	return map[string]string{
		dockerEditorLabel:                  installation,
		dockerEditorLabel + ".org":         scope.OrganizationID,
		dockerEditorLabel + ".workspace":   scope.WorkspaceID,
		dockerEditorLabel + ".task":        scope.TaskID,
		dockerEditorLabel + ".user":        metadata.OwnerID,
		dockerEditorLabel + ".generation":  generation,
		dockerEditorLabel + ".mode":        metadata.Mode,
		dockerEditorLabel + ".fingerprint": metadata.Fingerprint,
	}
}

func (e *ExecutorEngine) inspectEditor(ctx context.Context, lookup engine.Lookup, key engine.ResourceKey) (engine.Observation, error) {
	if e.editorClient == nil {
		return engine.Observation{}, engine.Unsupported("Docker editor lifecycle API")
	}
	id := lookup.Ref.ConcreteID
	if id == "" {
		if key.Scope.TaskID == "" {
			return engine.Observation{}, fmt.Errorf("%w: editor lookup needs a concrete ID or task scope", engine.ErrInvalidSpec)
		}
		id = editorName(e.installation, key.Scope)
	}
	result, err := e.editorClient.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return engine.Observation{State: engine.ObservationAbsent}, nil
		}
		return engine.Observation{}, err
	}
	resource, state, err := e.decodeEditor(result.Container, key)
	if err != nil {
		return engine.Observation{}, err
	}
	if state == engine.ObservationActive && e.config.NetworkName == "" {
		resource, err = e.inspectEditorRelay(ctx, resource)
		if err != nil {
			return engine.Observation{}, err
		}
	}
	return engine.Observation{State: state, Resource: resource}, nil
}

func (e *ExecutorEngine) decodeEditor(value container.InspectResponse, expected engine.ResourceKey) (engine.Resource, engine.ObservationState, error) {
	if value.Config == nil || value.Config.Labels[dockerEditorLabel] != e.installation {
		return engine.Resource{}, engine.ObservationAbsent, engine.ErrIdentityMismatch
	}
	labels := value.Config.Labels
	key := engine.ResourceKey{InstallationID: e.installation, Class: engine.ResourceClassEditor, Scope: engine.Scope{OrganizationID: labels[dockerEditorLabel+".org"], WorkspaceID: labels[dockerEditorLabel+".workspace"], TaskID: labels[dockerEditorLabel+".task"]}, ExecutionID: labels[dockerEditorLabel+".generation"], Qualifier: "task-editor"}
	metadata := engine.OwnershipMetadata{OwnerID: labels[dockerEditorLabel+".user"], Mode: labels[dockerEditorLabel+".mode"], Fingerprint: labels[dockerEditorLabel+".fingerprint"]}
	if key.Scope.OrganizationID == "" || key.Scope.WorkspaceID == "" || key.Scope.TaskID == "" || key.ExecutionID == "" || metadata.OwnerID == "" {
		return engine.Resource{}, engine.ObservationAbsent, engine.ErrIdentityMismatch
	}
	if expected.ExecutionID != "" && expected.ExecutionID != key.ExecutionID || expected.Scope.TaskID != "" && expected.Scope != key.Scope {
		return engine.Resource{}, engine.ObservationAbsent, engine.ErrIdentityMismatch
	}
	if value.State == nil {
		return engine.Resource{}, engine.ObservationAbsent, engine.ErrUncertain
	}
	state := engine.ObservationStopped
	resourceState := engine.ResourceStopped
	if value.State.Running || value.State.Restarting {
		state, resourceState = engine.ObservationActive, engine.ResourceRunning
	}
	resource := engine.Resource{Ref: engine.ResourceRef{Key: key, ConcreteID: value.ID}, State: resourceState, Metadata: metadata}
	if state == engine.ObservationActive {
		resource.Endpoint = "http://" + editorName(e.installation, key.Scope) + ":8080"
	}
	return resource, state, nil
}

func (e *ExecutorEngine) listEditors(ctx context.Context, selector engine.Selector) ([]engine.Resource, error) {
	e.editorMu.Lock()
	defer e.editorMu.Unlock()
	if e.editorClient == nil {
		return nil, engine.Unsupported("Docker editor lifecycle API")
	}
	listed, err := e.editorClient.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: client.Filters{}.Add("label", dockerEditorLabel+"="+e.installation)})
	if err != nil {
		return nil, err
	}
	resources := make([]engine.Resource, 0, len(listed.Items))
	for _, item := range listed.Items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if item.Labels[dockerEditorLabel+".kind"] == "relay" {
			continue
		}
		result, inspectErr := e.editorClient.ContainerInspect(ctx, item.ID, client.ContainerInspectOptions{})
		if errdefs.IsNotFound(inspectErr) {
			continue
		}
		if inspectErr != nil {
			return nil, inspectErr
		}
		resource, _, decodeErr := e.decodeEditor(result.Container, engine.ResourceKey{})
		if decodeErr != nil {
			return nil, decodeErr
		}
		if selector.Scope != nil && resource.Ref.Key.Scope != *selector.Scope {
			continue
		}
		resources = append(resources, resource)
	}
	if err = e.cleanupOrphanEditorRelays(ctx, listed.Items, resources); err != nil {
		return nil, err
	}
	if err = e.cleanupOrphanEditorNetworks(ctx, resources); err != nil {
		return nil, err
	}
	return resources, nil
}

func (e *ExecutorEngine) cleanupOrphanEditorNetworks(ctx context.Context, resources []engine.Resource) error {
	listed, err := e.editorClient.NetworkList(ctx, client.NetworkListOptions{Filters: client.Filters{}.Add("label", dockerEditorLabel+"="+e.installation)})
	if err != nil {
		return fmt.Errorf("list Docker editor networks: %w", err)
	}
	for _, item := range listed.Items {
		labels := item.Labels
		if labels[dockerEditorLabel] != e.installation {
			continue
		}
		key := engine.ResourceKey{InstallationID: e.installation, Class: engine.ResourceClassEditor, ExecutionID: labels[dockerEditorLabel+".generation"], Scope: engine.Scope{OrganizationID: labels[dockerEditorLabel+".org"], WorkspaceID: labels[dockerEditorLabel+".workspace"], TaskID: labels[dockerEditorLabel+".task"]}}
		metadata := engine.OwnershipMetadata{OwnerID: labels[dockerEditorLabel+".user"], Mode: labels[dockerEditorLabel+".mode"], Fingerprint: labels[dockerEditorLabel+".fingerprint"]}
		if key.ExecutionID == "" || key.Scope.OrganizationID == "" || key.Scope.WorkspaceID == "" || key.Scope.TaskID == "" || metadata.OwnerID == "" || item.Name != editorNetworkName(editorName(e.installation, key.Scope), key.ExecutionID) {
			return engine.ErrIdentityMismatch
		}
		owned := false
		for _, resource := range resources {
			if resource.Ref.Key.Scope == key.Scope && resource.Ref.Key.ExecutionID == key.ExecutionID {
				owned = true
				break
			}
		}
		if owned {
			continue
		}
		current, inspectErr := e.editorClient.ContainerInspect(ctx, editorName(e.installation, key.Scope), client.ContainerInspectOptions{})
		if inspectErr == nil {
			resource, _, decodeErr := e.decodeEditor(current.Container, engine.ResourceKey{})
			if decodeErr != nil {
				return decodeErr
			}
			if resource.Ref.Key.ExecutionID == key.ExecutionID {
				continue
			}
		} else if !errdefs.IsNotFound(inspectErr) {
			return inspectErr
		}
		if err = e.removeEditorNetwork(ctx, item.Name, key, metadata); err != nil {
			return err
		}
		manifestDirectory := filepath.Join(e.config.DataRoot, "editor-workspaces", editorName(e.installation, key.Scope), key.ExecutionID)
		if err = os.RemoveAll(manifestDirectory); err != nil {
			return err
		}
	}
	return nil
}

func (e *ExecutorEngine) stopEditor(ctx context.Context, ref engine.ResourceRef) error {
	if e.editorClient == nil || ref.ConcreteID == "" {
		return fmt.Errorf("%w: concrete Docker editor ID is required", engine.ErrInvalidSpec)
	}
	current, err := e.inspectEditor(ctx, engine.Lookup{Ref: ref}, ref.Key)
	if err != nil {
		return err
	}
	if current.State == engine.ObservationAbsent {
		if e.config.NetworkName == "" {
			return e.stopEditorRelay(ctx, ref.Key)
		}
		return nil
	}
	if e.config.NetworkName == "" {
		if err = e.stopEditorRelay(ctx, ref.Key); err != nil {
			return err
		}
	}
	if current.State == engine.ObservationActive {
		timeout := 5
		if _, err = e.editorClient.ContainerStop(ctx, current.Resource.Ref.ConcreteID, client.ContainerStopOptions{Timeout: &timeout}); err != nil && !errdefs.IsNotFound(err) {
			return err
		}
		confirmed, inspectErr := e.inspectEditor(ctx, engine.Lookup{Ref: ref}, ref.Key)
		if inspectErr != nil {
			return inspectErr
		}
		if confirmed.State == engine.ObservationActive {
			return engine.ErrUncertain
		}
	}
	return nil
}

func (e *ExecutorEngine) cleanupEditor(ctx context.Context, ref engine.ResourceRef) error {
	if e.editorClient == nil || ref.ConcreteID == "" {
		return fmt.Errorf("%w: concrete Docker editor ID is required", engine.ErrInvalidSpec)
	}
	observed, err := e.inspectEditor(ctx, engine.Lookup{Ref: ref}, ref.Key)
	if err != nil {
		return err
	}
	if observed.State == engine.ObservationAbsent {
		return e.cleanupEditorAncillary(ctx, ref.Key)
	}
	if observed.State == engine.ObservationActive {
		return errors.New("refusing to remove active Docker editor")
	}
	if _, err = e.editorClient.ContainerRemove(ctx, observed.Resource.Ref.ConcreteID, client.ContainerRemoveOptions{}); err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	return e.cleanupEditorAncillary(ctx, ref.Key)
}

func (e *ExecutorEngine) cleanupEditorAncillary(ctx context.Context, key engine.ResourceKey) error {
	name := editorName(e.installation, key.Scope)
	if e.config.NetworkName == "" {
		if err := e.stopEditorRelay(ctx, key); err != nil {
			return err
		}
	}
	networkName := editorNetworkName(name, key.ExecutionID)
	inspected, err := e.editorClient.NetworkInspect(ctx, networkName, client.NetworkInspectOptions{})
	if err == nil {
		labels := inspected.Network.Labels
		metadata := engine.OwnershipMetadata{OwnerID: labels[dockerEditorLabel+".user"], Mode: labels[dockerEditorLabel+".mode"], Fingerprint: labels[dockerEditorLabel+".fingerprint"]}
		if err = e.removeEditorNetwork(ctx, networkName, key, metadata); err != nil {
			return err
		}
	} else if !errdefs.IsNotFound(err) {
		return err
	}
	manifestDirectory := filepath.Join(e.config.DataRoot, "editor-workspaces", name, key.ExecutionID)
	return os.RemoveAll(manifestDirectory)
}

func (e *ExecutorEngine) removeEditorNetwork(ctx context.Context, name string, key engine.ResourceKey, metadata engine.OwnershipMetadata) error {
	inspected, err := e.editorClient.NetworkInspect(ctx, name, client.NetworkInspectOptions{})
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	labels := inspected.Network.Labels
	if inspected.Network.Name != name || inspected.Network.Driver != "bridge" || !inspected.Network.Internal || labels[dockerEditorLabel] != e.installation || labels[dockerEditorLabel+".generation"] != key.ExecutionID || labels[dockerEditorLabel+".org"] != key.Scope.OrganizationID || labels[dockerEditorLabel+".workspace"] != key.Scope.WorkspaceID || labels[dockerEditorLabel+".task"] != key.Scope.TaskID || labels[dockerEditorLabel+".user"] != metadata.OwnerID {
		return engine.ErrIdentityMismatch
	}
	for id := range inspected.Network.Containers {
		if _, err = e.editorClient.NetworkDisconnect(ctx, inspected.Network.ID, client.NetworkDisconnectOptions{Container: id, Force: true}); err != nil && !errdefs.IsNotFound(err) {
			return err
		}
	}
	if _, err = e.editorClient.NetworkRemove(ctx, inspected.Network.ID, client.NetworkRemoveOptions{}); err != nil && !errdefs.IsNotFound(err) {
		return err
	}
	return nil
}

func (e *ExecutorEngine) startEditorRelay(ctx context.Context, spec engine.LaunchSpec, privateNetwork string, labels map[string]string) (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	token := hex.EncodeToString(secret)
	labels[dockerEditorLabel+".kind"] = "relay"
	port := network.MustParsePort("8081/tcp")
	pids := int64(32)
	relayName := privateNetwork + "-relay"
	created, err := e.editorClient.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: relayName,
		Config: &container.Config{
			Image: spec.Artifact.ContainerImage, User: fmt.Sprintf("%d:%d", spec.Security.RunAsUser, spec.Security.RunAsGroup),
			Entrypoint: []string{"/opt/code-server/lib/node", "/opt/editor/relay.cjs"},
			Env:        []string{editorRelayTokenEnv + "=" + token, "ORKEST_EDITOR_RELAY_UPSTREAM=http://" + editorName(e.installation, spec.Key.Scope) + ":8080"},
			Labels:     labels, ExposedPorts: network.PortSet{port: {}},
		},
		HostConfig: &container.HostConfig{
			NetworkMode: "bridge", ReadonlyRootfs: true, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"},
			Resources:     container.Resources{Memory: 128 << 20, NanoCPUs: 1_000_000_000, PidsLimit: &pids},
			PortBindings:  network.PortMap{port: {{HostIP: netip.MustParseAddr("127.0.0.1")}}},
			RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyDisabled},
		},
	})
	if err != nil {
		return "", fmt.Errorf("create Docker editor relay: %w", err)
	}
	if _, err = e.editorClient.NetworkConnect(ctx, privateNetwork, client.NetworkConnectOptions{Container: created.ID}); err != nil {
		_, _ = e.editorClient.ContainerRemove(context.WithoutCancel(ctx), created.ID, client.ContainerRemoveOptions{Force: true})
		return "", fmt.Errorf("connect Docker editor relay: %w", err)
	}
	if _, err = e.editorClient.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		return "", fmt.Errorf("start Docker editor relay: %w", err)
	}
	return token, nil
}

func (e *ExecutorEngine) inspectEditorRelay(ctx context.Context, resource engine.Resource) (engine.Resource, error) {
	privateNetwork := editorNetworkName(editorName(e.installation, resource.Ref.Key.Scope), resource.Ref.Key.ExecutionID)
	result, err := e.editorClient.ContainerInspect(ctx, privateNetwork+"-relay", client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		return resource, nil
	}
	if err != nil {
		return resource, err
	}
	if result.Container.Config == nil {
		return resource, engine.ErrUncertain
	}
	labels := result.Container.Config.Labels
	if labels[dockerEditorLabel] != e.installation || labels[dockerEditorLabel+".kind"] != "relay" || labels[dockerEditorLabel+".generation"] != resource.Ref.Key.ExecutionID || labels[dockerEditorLabel+".org"] != resource.Ref.Key.Scope.OrganizationID || labels[dockerEditorLabel+".workspace"] != resource.Ref.Key.Scope.WorkspaceID || labels[dockerEditorLabel+".task"] != resource.Ref.Key.Scope.TaskID {
		return resource, engine.ErrIdentityMismatch
	}
	if result.Container.State == nil || !result.Container.State.Running {
		return resource, nil
	}
	for _, variable := range result.Container.Config.Env {
		if value, found := strings.CutPrefix(variable, editorRelayTokenEnv+"="); found {
			resource.Access = value
		}
	}
	if len(resource.Access) != 64 {
		return resource, errors.New("docker editor relay credentials are unavailable")
	}
	resource.Endpoint, err = publishedEndpoint(result.Container, 8081)
	return resource, err
}

func (e *ExecutorEngine) stopEditorRelay(ctx context.Context, key engine.ResourceKey) error {
	privateNetwork := editorNetworkName(editorName(e.installation, key.Scope), key.ExecutionID)
	result, err := e.editorClient.ContainerInspect(ctx, privateNetwork+"-relay", client.ContainerInspectOptions{})
	if errdefs.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if result.Container.Config == nil {
		return engine.ErrUncertain
	}
	labels := result.Container.Config.Labels
	if labels[dockerEditorLabel] != e.installation || labels[dockerEditorLabel+".kind"] != "relay" || labels[dockerEditorLabel+".generation"] != key.ExecutionID || labels[dockerEditorLabel+".org"] != key.Scope.OrganizationID || labels[dockerEditorLabel+".workspace"] != key.Scope.WorkspaceID || labels[dockerEditorLabel+".task"] != key.Scope.TaskID {
		return engine.ErrIdentityMismatch
	}
	if result.Container.State != nil && result.Container.State.Running {
		timeout := 3
		if _, err = e.editorClient.ContainerStop(ctx, result.Container.ID, client.ContainerStopOptions{Timeout: &timeout}); err != nil && !errdefs.IsNotFound(err) {
			return err
		}
		result, err = e.editorClient.ContainerInspect(ctx, result.Container.ID, client.ContainerInspectOptions{})
		if errdefs.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if result.Container.State == nil || result.Container.State.Running {
			return engine.ErrUncertain
		}
	}
	if _, err = e.editorClient.ContainerRemove(ctx, result.Container.ID, client.ContainerRemoveOptions{}); errdefs.IsNotFound(err) {
		return nil
	}
	return err
}

func (e *ExecutorEngine) cleanupOrphanEditorRelays(ctx context.Context, containers []container.Summary, resources []engine.Resource) error {
	for _, item := range containers {
		labels := item.Labels
		if labels[dockerEditorLabel+".kind"] != "relay" || labels[dockerEditorLabel] != e.installation {
			continue
		}
		key := engine.ResourceKey{Scope: engine.Scope{OrganizationID: labels[dockerEditorLabel+".org"], WorkspaceID: labels[dockerEditorLabel+".workspace"], TaskID: labels[dockerEditorLabel+".task"]}, ExecutionID: labels[dockerEditorLabel+".generation"]}
		if key.Scope.OrganizationID == "" || key.Scope.WorkspaceID == "" || key.Scope.TaskID == "" || key.ExecutionID == "" {
			return engine.ErrIdentityMismatch
		}
		owned := false
		for _, resource := range resources {
			if resource.Ref.Key.Scope == key.Scope && resource.Ref.Key.ExecutionID == key.ExecutionID {
				owned = true
				break
			}
		}
		if !owned {
			current, inspectErr := e.editorClient.ContainerInspect(ctx, editorName(e.installation, key.Scope), client.ContainerInspectOptions{})
			if inspectErr == nil {
				resource, _, decodeErr := e.decodeEditor(current.Container, engine.ResourceKey{})
				if decodeErr != nil {
					return decodeErr
				}
				if resource.Ref.Key.ExecutionID == key.ExecutionID {
					continue
				}
			} else if !errdefs.IsNotFound(inspectErr) {
				return inspectErr
			}
			if err := e.stopEditorRelay(ctx, key); err != nil {
				return err
			}
		}
	}
	return nil
}
