package kubernetes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	"github.com/josemiguelmelo/orkest/internal/platform/agentctllogs"
)

const agentctlPort = 8081

type EngineConfig struct {
	DataRoot, DataSubpath, Namespace, InstallationID, DataPVC string
	ServiceAccount, CodexAuthSecret, CodexAuthSecretKey       string
	ImagePullSecrets                                          []string
	CPURequest, MemoryRequest, CPULimit, MemoryLimit          string
	UID, GID, TerminationGraceSeconds                         int64
	StartupTimeout                                            time.Duration
	Client                                                    API
}

// ExecutorEngine runs agent workloads in an operator-managed namespace and PVC.
type ExecutorEngine struct {
	config    EngineConfig
	client    API
	workspace *WorkspaceResolver
	ctx       context.Context
	cancel    context.CancelFunc
}

var _ engine.ExecutorEngine = (*ExecutorEngine)(nil)

func New(config EngineConfig) (*ExecutorEngine, error) {
	if config.Client == nil {
		return nil, errors.New("kubernetes engine API client is required")
	}
	for field, value := range map[string]string{"namespace": config.Namespace, "installation ID": config.InstallationID, "data PVC": config.DataPVC} {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("kubernetes engine %s is required", field)
		}
	}
	for field, value := range map[string]string{"namespace": config.Namespace, "installation ID": config.InstallationID, "data PVC": config.DataPVC} {
		if !dnsLabel(value) {
			return nil, fmt.Errorf("kubernetes engine %s must be a DNS label", field)
		}
	}
	if config.ServiceAccount != "" && !dnsLabel(config.ServiceAccount) {
		return nil, errors.New("kubernetes engine service account must be a DNS label")
	}
	if config.CodexAuthSecret != "" && (!dnsLabel(config.CodexAuthSecret) || !validSecretKey(config.CodexAuthSecretKey)) {
		return nil, errors.New("kubernetes engine Codex Secret name and key are invalid")
	}
	if config.CodexAuthSecret == "" && config.CodexAuthSecretKey != "" {
		return nil, errors.New("kubernetes engine Codex Secret name and key must be configured together")
	}
	for _, secret := range config.ImagePullSecrets {
		if !dnsLabel(secret) {
			return nil, fmt.Errorf("kubernetes engine image-pull Secret %q must be a DNS label", secret)
		}
	}
	root, err := filepath.Abs(config.DataRoot)
	if err != nil || strings.TrimSpace(config.DataRoot) == "" {
		return nil, errors.New("kubernetes engine data root is required and must be resolvable")
	}
	if config.DataSubpath != "" && (filepath.IsAbs(config.DataSubpath) || filepath.Clean(config.DataSubpath) == ".." || strings.HasPrefix(filepath.Clean(config.DataSubpath), ".."+string(filepath.Separator))) {
		return nil, errors.New("kubernetes engine PVC subpath must be relative and contained")
	}
	if config.UID <= 0 || config.GID <= 0 {
		return nil, errors.New("kubernetes engine worker UID and GID must be positive")
	}
	if config.StartupTimeout <= 0 {
		config.StartupTimeout = 2 * time.Minute
	}
	if config.TerminationGraceSeconds <= 0 {
		config.TerminationGraceSeconds = 10
	}
	if config.CPURequest == "" {
		config.CPURequest = "250m"
	}
	if config.MemoryRequest == "" {
		config.MemoryRequest = "512Mi"
	}
	if config.CPULimit == "" {
		config.CPULimit = "2"
	}
	if config.MemoryLimit == "" {
		config.MemoryLimit = "4Gi"
	}
	workspace, err := NewWorkspaceResolver(root)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	config.DataRoot = root
	return &ExecutorEngine{config: config, client: config.Client, workspace: workspace, ctx: ctx, cancel: cancel}, nil
}

func (e *ExecutorEngine) Capabilities() engine.Capabilities {
	return engine.Capabilities{Modes: []engine.ExecutionMode{engine.ExecutionModeContainer}, DurableWork: true, DurableState: true, PlatformWorkspaceSetup: true, CredentialBindings: e.config.CodexAuthSecret != ""}
}

func (e *ExecutorEngine) Validate(ctx context.Context, requirements engine.Requirements) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return engine.ValidateCapabilities(e.Capabilities(), requirements)
}

func (e *ExecutorEngine) ResolveWorkspace(ctx context.Context, request engine.WorkspaceRequest) (engine.Workspace, error) {
	return e.workspace.ResolveWorkspace(ctx, request)
}

func (e *ExecutorEngine) Start(ctx context.Context, spec engine.LaunchSpec) (engine.Resource, error) {
	resource := engine.Resource{Ref: engine.ResourceRef{Key: spec.Key}}
	if err := engine.ValidateSpec(spec); err != nil {
		return resource, err
	}
	if err := e.Validate(ctx, spec.Requirements); err != nil {
		return resource, err
	}
	if spec.Class != engine.ResourceClassAgent || spec.Mode != engine.ExecutionModeContainer {
		return resource, engine.Unsupported("kubernetes engine currently supports container agent workloads")
	}
	if spec.Key.InstallationID != "" && spec.Key.InstallationID != e.config.InstallationID {
		return resource, engine.ErrIdentityMismatch
	}
	if len(spec.Mounts) != 0 || len(spec.GeneratedFiles) != 0 || len(spec.TemporaryMounts) != 0 || spec.HostEnvironment.InheritAllowlisted {
		return resource, engine.Unsupported("kubernetes agent mount or host-environment requirement")
	}
	workspace, err := e.ResolveWorkspace(ctx, engine.WorkspaceRequest{Scope: spec.Key.Scope, Intent: engine.WorkspaceInspect, Required: engine.WorkspaceGuarantees{DurableWork: true, DurableState: true, PlatformSetup: true}})
	if err != nil {
		return resource, err
	}
	if !workspace.Exists || workspace.WorkPath != spec.Workspace.WorkPath || workspace.StatePath != spec.Workspace.StatePath {
		return resource, fmt.Errorf("%w: Kubernetes PVC workspace changed before launch", engine.ErrIdentityMismatch)
	}
	if spec.Artifact.ContainerImage == "" {
		return resource, fmt.Errorf("%w: Kubernetes agent image is required", engine.ErrInvalidSpec)
	}
	name := resourceName(e.config.InstallationID, spec.Key.ExecutionID)
	resource.Ref.ConcreteID = e.processID(name, "")
	labels := e.labels(spec.Key, spec.Artifact.ContainerImage, spec.Key.Qualifier)
	service := map[string]any{"apiVersion": "v1", "kind": "Service", "metadata": map[string]any{"name": name, "namespace": e.config.Namespace, "labels": labels}, "spec": map[string]any{"type": "ClusterIP", "selector": map[string]string{"orkest.run": labels["orkest.run"]}, "ports": []any{map[string]any{"name": "agentctl", "port": agentctlPort, "targetPort": agentctlPort, "protocol": "TCP"}}}}
	if _, err = e.ensureResource(ctx, e.resourcePath("services", name), service, labels); err != nil {
		return resource, fmt.Errorf("create Kubernetes agentctl Service: %w", err)
	}
	useCodex := false
	for _, binding := range spec.CredentialBindings {
		if binding.Name != "codex-auth" || e.config.CodexAuthSecret == "" || binding.Target != "/agentctl-state/codex-auth/auth.json" {
			return resource, engine.Unsupported("kubernetes credential binding is unavailable")
		}
		useCodex = true
	}
	if useCodex && !e.Capabilities().CredentialBindings {
		return resource, engine.Unsupported("kubernetes Codex Secret binding is unavailable")
	}
	containerEnv, secretValues := environmentForPod(spec.Environment)
	if useCodex {
		delete(secretValues, "ORKEST_CODEX_AUTH_FILE")
	}
	if len(secretValues) != 0 {
		secretName := name + "-env"
		secret := map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": secretName, "namespace": e.config.Namespace, "labels": labels}, "type": "Opaque", "stringData": secretValues}
		if _, err = e.ensureResource(ctx, e.resourcePath("secrets", secretName), secret, labels); err != nil {
			return resource, fmt.Errorf("create Kubernetes agentctl environment Secret: %w", err)
		}
		for key := range secretValues {
			containerEnv = append(containerEnv, map[string]any{"name": key, "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": secretName, "key": key}}})
		}
	}
	for _, binding := range spec.CredentialBindings {
		containerEnv = append(containerEnv, map[string]any{"name": "ORKEST_CODEX_AUTH_FILE", "value": binding.Target})
	}
	args, err := resolveArguments(spec.Arguments)
	if err != nil {
		return resource, err
	}
	base := strings.Trim(filepath.ToSlash(filepath.Join("tasks", workspace.CompatibilityID)), "/")
	if e.config.DataSubpath != "" {
		base = pathJoin(e.config.DataSubpath, base)
	}
	uid, gid := e.config.UID, e.config.GID
	security := map[string]any{"runAsNonRoot": true, "runAsUser": uid, "runAsGroup": gid, "allowPrivilegeEscalation": false, "readOnlyRootFilesystem": false, "capabilities": map[string]any{"drop": []string{"ALL"}}}
	volumeMounts := []any{map[string]any{"name": "task-work", "mountPath": "/workspace", "subPath": base + "/work"}, map[string]any{"name": "task-state", "mountPath": "/agentctl-state", "subPath": base + "/agentctl-state"}}
	volumes := []any{map[string]any{"name": "task-work", "persistentVolumeClaim": map[string]any{"claimName": e.config.DataPVC, "readOnly": false}}, map[string]any{"name": "task-state", "persistentVolumeClaim": map[string]any{"claimName": e.config.DataPVC, "readOnly": false}}}
	if useCodex {
		volumes = append(volumes, map[string]any{"name": "codex-auth", "secret": map[string]any{"secretName": e.config.CodexAuthSecret, "items": []any{map[string]any{"key": e.config.CodexAuthSecretKey, "path": "auth.json"}}}})
		volumeMounts = append(volumeMounts, map[string]any{"name": "codex-auth", "mountPath": "/agentctl-state/codex-auth", "readOnly": true})
	}
	resources := map[string]any{"requests": map[string]string{"cpu": e.config.CPURequest, "memory": e.config.MemoryRequest}, "limits": map[string]string{"cpu": e.config.CPULimit, "memory": e.config.MemoryLimit}}
	container := map[string]any{"name": "agentctl", "image": spec.Artifact.ContainerImage, "args": args, "env": containerEnv, "ports": []any{map[string]any{"name": "agentctl", "containerPort": agentctlPort, "protocol": "TCP"}}, "readinessProbe": map[string]any{"httpGet": map[string]any{"path": "/health", "port": "agentctl"}, "periodSeconds": 2, "failureThreshold": 90}, "securityContext": security, "volumeMounts": volumeMounts, "resources": resources}
	podSpec := map[string]any{"restartPolicy": "Never", "automountServiceAccountToken": false, "terminationGracePeriodSeconds": e.config.TerminationGraceSeconds, "securityContext": map[string]any{"runAsNonRoot": true, "runAsUser": uid, "runAsGroup": gid}, "containers": []any{container}, "volumes": volumes}
	if e.config.ServiceAccount != "" {
		podSpec["serviceAccountName"] = e.config.ServiceAccount
	}
	if len(e.config.ImagePullSecrets) != 0 {
		refs := make([]any, 0, len(e.config.ImagePullSecrets))
		for _, secret := range e.config.ImagePullSecrets {
			refs = append(refs, map[string]any{"name": secret})
		}
		podSpec["imagePullSecrets"] = refs
	}
	pod := map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": name, "namespace": e.config.Namespace, "labels": labels}, "spec": podSpec}
	created, err := e.ensureResource(ctx, e.resourcePath("pods", name), pod, labels)
	if err != nil {
		return resource, fmt.Errorf("create Kubernetes agentctl Pod: %w", err)
	}
	uidValue, _ := nestedString(created, "metadata", "uid")
	resource.Ref.ConcreteID = e.processID(name, uidValue)
	waitCtx, cancel := context.WithTimeout(ctx, e.config.StartupTimeout)
	defer cancel()
	for {
		current, status, getErr := e.client.Do(waitCtx, http.MethodGet, e.resourcePath("pods", name), nil)
		if getErr != nil {
			return resource, fmt.Errorf("inspect Kubernetes agentctl Pod: %w", getErr)
		}
		if status == http.StatusNotFound {
			return resource, errors.New("agentctl Pod disappeared during startup")
		}
		phase, _ := nestedString(current, "status", "phase")
		if phase == "Running" && boolAt(current, "status", "containerStatuses", 0, "ready") {
			resource.State = engine.ResourceRunning
			resource.Endpoint = fmt.Sprintf("http://%s.%s.svc:%d", name, e.config.Namespace, agentctlPort)
			go e.forwardLogs(name, spec.Key.ExecutionID)
			return resource, nil
		}
		if phase == "Failed" || phase == "Succeeded" {
			reason, _ := nestedString(current, "status", "reason")
			return resource, fmt.Errorf("agentctl Pod exited during startup (%s)", reason)
		}
		select {
		case <-waitCtx.Done():
			return resource, fmt.Errorf("wait for agentctl Pod readiness: %w", waitCtx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (e *ExecutorEngine) Inspect(ctx context.Context, lookup engine.Lookup) (engine.Observation, error) {
	key := lookup.Ref.Key
	if lookup.Key != nil {
		key = *lookup.Key
	}
	if key.InstallationID != "" && key.InstallationID != e.config.InstallationID {
		return engine.Observation{}, engine.ErrIdentityMismatch
	}
	if key.Class != engine.ResourceClassUnknown && key.Class != engine.ResourceClassAgent {
		return engine.Observation{}, engine.ErrIdentityMismatch
	}
	name, uid := "", ""
	if lookup.Ref.ConcreteID != "" {
		var err error
		name, uid, err = e.parseID(lookup.Ref.ConcreteID)
		if err != nil {
			return engine.Observation{}, err
		}
	} else if key.ExecutionID != "" {
		name = resourceName(e.config.InstallationID, key.ExecutionID)
	} else {
		return engine.Observation{}, fmt.Errorf("%w: Kubernetes lookup requires execution or concrete identity", engine.ErrInvalidSpec)
	}
	pod, status, err := e.client.Do(ctx, http.MethodGet, e.resourcePath("pods", name), nil)
	if err != nil {
		return engine.Observation{}, fmt.Errorf("inspect Kubernetes workload: %w", err)
	}
	if status == http.StatusNotFound {
		return engine.Observation{State: engine.ObservationAbsent}, nil
	}
	labels, _ := nestedMap(pod, "metadata", "labels")
	if labels["app.kubernetes.io/managed-by"] != "orkest" || labels["orkest.installation"] != e.config.InstallationID {
		return engine.Observation{}, engine.ErrIdentityMismatch
	}
	if uid != "" && nestedStringValue(pod, "metadata", "uid") != uid {
		return engine.Observation{}, engine.ErrIdentityMismatch
	}
	if key.ExecutionID != "" && labels["orkest.run"] != hashShort(key.ExecutionID) {
		return engine.Observation{}, engine.ErrIdentityMismatch
	}
	if key.Scope.OrganizationID != "" && labels["orkest.scope"] != scopeHash(key.Scope) {
		return engine.Observation{}, engine.ErrIdentityMismatch
	}
	if key.Qualifier != "" && labels["orkest.runtime"] != key.Qualifier {
		return engine.Observation{}, engine.ErrIdentityMismatch
	}
	phase, _ := nestedString(pod, "status", "phase")
	state := engine.ObservationActive
	resourceState := engine.ResourceRunning
	if phase == "Succeeded" || phase == "Failed" {
		state, resourceState = engine.ObservationStopped, engine.ResourceStopped
	}
	if phase == "" || phase == "Pending" || phase == "Unknown" {
		return engine.Observation{State: engine.ObservationActive, Resource: engine.Resource{Ref: engine.ResourceRef{Key: mergeKey(key, e.resourceKey(labels), e.config.InstallationID), ConcreteID: e.processID(name, nestedStringValue(pod, "metadata", "uid"))}, State: engine.ResourceStarting}}, nil
	}
	resource := engine.Resource{Ref: engine.ResourceRef{Key: mergeKey(key, e.resourceKey(labels), e.config.InstallationID), ConcreteID: e.processID(name, nestedStringValue(pod, "metadata", "uid"))}, State: resourceState}
	if key.ExecutionID != "" {
		resource.Ref.Key.ExecutionID = key.ExecutionID
	}
	return engine.Observation{State: state, Resource: resource}, nil
}

func (e *ExecutorEngine) List(ctx context.Context, selector engine.Selector) ([]engine.Resource, error) {
	if selector.InstallationID != "" && selector.InstallationID != e.config.InstallationID {
		return nil, engine.ErrIdentityMismatch
	}
	// This engine version has never created editor workloads. Report confirmed
	// absence so the shared editor policy can keep the API available while its
	// stricter launch requirements remain unsupported.
	if selector.Class == engine.ResourceClassEditor {
		return []engine.Resource{}, nil
	}
	if selector.Class != engine.ResourceClassAgent {
		return nil, fmt.Errorf("%w: Kubernetes list requires agent resource class", engine.ErrInvalidSpec)
	}
	response, status, err := e.client.Do(ctx, http.MethodGet, e.resourcePath("pods", ""), nil)
	if err != nil {
		return nil, fmt.Errorf("list Kubernetes agent workloads: %w", err)
	}
	if status >= 300 {
		return nil, fmt.Errorf("list Kubernetes agent workloads returned HTTP %d", status)
	}
	items, _ := response["items"].([]any)
	resources := make([]engine.Resource, 0, len(items))
	for _, item := range items {
		pod, ok := item.(map[string]any)
		if !ok {
			continue
		}
		labels, _ := nestedMap(pod, "metadata", "labels")
		if labels["app.kubernetes.io/managed-by"] != "orkest" || labels["orkest.installation"] != e.config.InstallationID {
			continue
		}
		if selector.Scope != nil && labels["orkest.scope"] != scopeHash(*selector.Scope) {
			continue
		}
		name, _ := nestedString(pod, "metadata", "name")
		uid := nestedStringValue(pod, "metadata", "uid")
		observation, inspectErr := e.Inspect(ctx, engine.Lookup{Ref: engine.ResourceRef{ConcreteID: e.processID(name, uid)}})
		if inspectErr != nil {
			if errors.Is(inspectErr, engine.ErrIdentityMismatch) {
				continue
			}
			return nil, inspectErr
		}
		if observation.State != engine.ObservationAbsent {
			resources = append(resources, observation.Resource)
		}
	}
	return resources, nil
}

func (e *ExecutorEngine) Stop(ctx context.Context, ref engine.ResourceRef) error {
	if ref.Key.InstallationID != "" && ref.Key.InstallationID != e.config.InstallationID {
		return engine.ErrIdentityMismatch
	}
	var name, uid string
	if ref.ConcreteID != "" {
		var err error
		name, uid, err = e.parseID(ref.ConcreteID)
		if err != nil {
			return err
		}
	} else if ref.Key.ExecutionID != "" {
		name = resourceName(e.config.InstallationID, ref.Key.ExecutionID)
	} else {
		return fmt.Errorf("%w: Kubernetes stop requires execution or concrete identity", engine.ErrInvalidSpec)
	}
	pod, status, err := e.client.Do(ctx, http.MethodGet, e.resourcePath("pods", name), nil)
	if err != nil {
		return fmt.Errorf("inspect Kubernetes Pod before termination: %w", err)
	}
	if status == http.StatusNotFound {
		if err = e.deleteOwned(ctx, "secrets", name+"-env"); err != nil {
			return err
		}
		return e.deleteOwned(ctx, "services", name)
	}
	labels, _ := nestedMap(pod, "metadata", "labels")
	if labels["app.kubernetes.io/managed-by"] != "orkest" || labels["orkest.installation"] != e.config.InstallationID {
		return engine.ErrIdentityMismatch
	}
	if ref.Key.ExecutionID != "" && labels["orkest.run"] != hashShort(ref.Key.ExecutionID) {
		return engine.ErrIdentityMismatch
	}
	if ref.Key.Scope.OrganizationID != "" && labels["orkest.scope"] != scopeHash(ref.Key.Scope) {
		return engine.ErrIdentityMismatch
	}
	if ref.Key.Qualifier != "" && labels["orkest.runtime"] != ref.Key.Qualifier {
		return engine.ErrIdentityMismatch
	}
	actualUID := nestedStringValue(pod, "metadata", "uid")
	if uid != "" && uid != actualUID {
		return engine.ErrIdentityMismatch
	}
	options := map[string]any{"apiVersion": "v1", "kind": "DeleteOptions", "gracePeriodSeconds": e.config.TerminationGraceSeconds}
	if actualUID != "" {
		options["preconditions"] = map[string]string{"uid": actualUID}
	}
	_, status, err = e.client.Do(ctx, http.MethodDelete, e.resourcePath("pods", name), options)
	if err != nil || status >= 300 && status != http.StatusNotFound {
		return fmt.Errorf("delete Kubernetes Pod: HTTP %d: %w", status, err)
	}
	for {
		_, status, err = e.client.Do(ctx, http.MethodGet, e.resourcePath("pods", name), nil)
		if err != nil {
			return fmt.Errorf("confirm Kubernetes Pod termination: %w", err)
		}
		if status == http.StatusNotFound {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for Kubernetes Pod termination: %w", ctx.Err())
		case <-time.After(300 * time.Millisecond):
		}
	}
	if err = e.deleteOwned(ctx, "secrets", name+"-env"); err != nil {
		return err
	}
	return e.deleteOwned(ctx, "services", name)
}

func (e *ExecutorEngine) Cleanup(ctx context.Context, ref engine.ResourceRef) error {
	return e.Stop(ctx, ref)
}

func (e *ExecutorEngine) Close() error { e.cancel(); return nil }

func (e *ExecutorEngine) forwardLogs(name, executionID string) {
	logs, ok := e.client.(LogAPI)
	if !ok {
		return
	}
	stream, err := logs.PodLogs(e.ctx, e.resourcePath("pods", name)+"/log?container=agentctl&follow=true")
	if err != nil {
		return
	}
	defer func() { _ = stream.Close() }()
	agentctllogs.Forward(stream, "pod", executionID)
}

func (e *ExecutorEngine) labels(key engine.ResourceKey, image, runtime string) map[string]string {
	return map[string]string{"app.kubernetes.io/managed-by": "orkest", "orkest.installation": e.config.InstallationID, "orkest.run": hashShort(key.ExecutionID), "orkest.scope": scopeHash(key.Scope), "orkest.runtime": runtime, "orkest.image": hashShort(image)}
}

func (e *ExecutorEngine) resourcePath(resource, name string) string {
	base := "/api/v1/namespaces/" + e.config.Namespace + "/" + resource
	if name != "" {
		base += "/" + name
	}
	return base
}

func (e *ExecutorEngine) processID(name, uid string) string {
	return e.config.Namespace + "/" + name + "/" + uid
}

func (e *ExecutorEngine) parseID(id string) (string, string, error) {
	parts := strings.Split(id, "/")
	if len(parts) != 3 || parts[0] != e.config.Namespace || !dnsLabel(parts[1]) {
		return "", "", errors.New("invalid Kubernetes process identity")
	}
	return parts[1], parts[2], nil
}

func (e *ExecutorEngine) ensureResource(ctx context.Context, requestPath string, desired map[string]any, labels map[string]string) (map[string]any, error) {
	existing, status, err := e.client.Do(ctx, http.MethodGet, requestPath, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		created, code, createErr := e.client.Do(ctx, http.MethodPost, strings.TrimSuffix(requestPath, "/"+lastPart(requestPath)), desired)
		if createErr == nil && code < 300 {
			return created, nil
		}
		existing, status, err = e.client.Do(ctx, http.MethodGet, requestPath, nil)
		if err != nil || status >= 300 {
			return nil, errors.Join(createErr, err)
		}
		if verifyLabels(existing, labels, lastPart(requestPath)) != nil {
			return nil, engine.ErrIdentityMismatch
		}
		return existing, nil
	}
	if status >= 300 {
		return nil, fmt.Errorf("kubernetes resource read returned HTTP %d", status)
	}
	if err = verifyLabels(existing, labels, lastPart(requestPath)); err != nil {
		return nil, err
	}
	return existing, nil
}

func (e *ExecutorEngine) deleteOwned(ctx context.Context, resource, name string) error {
	path := e.resourcePath(resource, name)
	value, status, err := e.client.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	if nestedStringValue(value, "metadata", "labels", "orkest.installation") != e.config.InstallationID {
		return engine.ErrIdentityMismatch
	}
	_, status, err = e.client.Do(ctx, http.MethodDelete, path, map[string]any{"apiVersion": "v1", "kind": "DeleteOptions"})
	if err != nil || status >= 300 && status != http.StatusNotFound {
		return fmt.Errorf("delete Kubernetes %s: HTTP %d: %w", resource, status, err)
	}
	return nil
}

func environmentForPod(values map[string]string) ([]any, map[string]string) {
	env := make([]any, 0, len(values))
	secrets := make(map[string]string, len(values))
	for name, value := range values {
		secrets[name] = value
	}
	return env, secrets
}

func resolveArguments(arguments []engine.Argument) ([]string, error) {
	values := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		if argument.Kind == engine.ArgumentLiteral {
			values = append(values, argument.Value)
			continue
		}
		if argument.Kind != engine.ArgumentEndpointAddress || argument.Endpoint != "agentctl" {
			return nil, engine.Unsupported("kubernetes argument endpoint binding")
		}
		values = append(values, fmt.Sprintf("0.0.0.0:%d", agentctlPort))
	}
	return values, nil
}

func resourceName(installation, run string) string {
	hash := sha256.Sum256([]byte(installation + "/" + run))
	return "orkest-" + installation[:min(8, len(installation))] + "-" + hex.EncodeToString(hash[:])[:32]
}

func hashShort(value string) string {
	hash := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", hash)[:32]
}

func scopeHash(scope engine.Scope) string {
	return hashShort(scope.OrganizationID + "/" + scope.WorkspaceID + "/" + scope.TaskID)
}

func (e *ExecutorEngine) resourceKey(labels map[string]any) engine.ResourceKey {
	return engine.ResourceKey{InstallationID: e.config.InstallationID, Class: engine.ResourceClassAgent, Scope: engine.Scope{OrganizationID: nestedStringValue(labels, "orkest.scope")}, ExecutionID: nestedStringValue(labels, "orkest.run"), Qualifier: nestedStringValue(labels, "orkest.runtime")}
}

func mergeKey(expected, observed engine.ResourceKey, installation string) engine.ResourceKey {
	if expected.Class != engine.ResourceClassUnknown {
		observed.Class = expected.Class
	}
	if expected.Scope.OrganizationID != "" {
		observed.Scope = expected.Scope
	}
	if expected.ExecutionID != "" {
		observed.ExecutionID = expected.ExecutionID
	}
	if expected.Qualifier != "" {
		observed.Qualifier = expected.Qualifier
	}
	observed.InstallationID = installation
	return observed
}

func verifyLabels(resource map[string]any, labels map[string]string, name string) error {
	existing, _ := nestedMap(resource, "metadata", "labels")
	for key, value := range labels {
		if existing[key] != value {
			return fmt.Errorf("%w: refusing to adopt Kubernetes resource %s with mismatched ownership label %s", engine.ErrIdentityMismatch, name, key)
		}
	}
	return nil
}

func lastPart(value string) string { parts := strings.Split(value, "/"); return parts[len(parts)-1] }

func nested(value map[string]any, keys ...string) any {
	var current any = value
	for _, key := range keys {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = mapping[key]
	}
	return current
}

func nestedString(value map[string]any, keys ...string) (string, bool) {
	result, ok := nested(value, keys...).(string)
	return result, ok
}

func nestedStringValue(value map[string]any, keys ...string) string {
	result, _ := nestedString(value, keys...)
	return result
}

func nestedMap(value map[string]any, keys ...string) (map[string]any, bool) {
	result, ok := nested(value, keys...).(map[string]any)
	return result, ok
}

func boolAt(value map[string]any, keys ...any) bool {
	var current any = value
	for _, key := range keys {
		switch part := key.(type) {
		case string:
			mapping, ok := current.(map[string]any)
			if !ok {
				return false
			}
			current = mapping[part]
		case int:
			items, ok := current.([]any)
			if !ok || part >= len(items) {
				return false
			}
			current = items[part]
		}
	}
	result, _ := current.(bool)
	return result
}

func dnsLabel(value string) bool {
	if len(value) < 1 || len(value) > 63 || !asciiLowerOrDigit(rune(value[0])) {
		return false
	}
	for _, char := range value {
		if !asciiLowerOrDigit(char) && char != '-' {
			return false
		}
	}
	return value[len(value)-1] != '-'
}

func asciiLowerOrDigit(value rune) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}

func validSecretKey(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		valid := asciiLowerOrDigit(char) || char >= 'A' && char <= 'Z' || char == '.' || char == '_' || char == '-'
		if !valid {
			return false
		}
	}
	return true
}

func pathJoin(left, right string) string {
	return strings.Trim(filepath.ToSlash(filepath.Join(left, right)), "/")
}
