// Package kubernetes runs independent agentctl Pods on an operator-managed cluster.
package kubernetes

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/checkout"
	runtimecatalog "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/platform/agentctllogs"
	"github.com/josemiguelmelo/orkest/internal/server/coderepository"
	"github.com/josemiguelmelo/orkest/internal/server/orchestration"
	model "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
)

const agentctlPort = 8081

// API is the narrow Kubernetes REST surface used by this adapter. It also makes
// lifecycle behavior independently testable without importing Kubernetes SDK types.
type API interface {
	Do(context.Context, string, string, any) (map[string]any, int, error)
}

type logAPI interface {
	PodLogs(context.Context, string) (io.ReadCloser, error)
}

type Config struct {
	DataRoot, Namespace, InstallationID, DataPVC, DataSubpath string
	ServiceAccount                                            string
	CodexAuthSecret, CodexAuthSecretKey                       string
	ImagePullSecrets                                          []string
	CPURequest, MemoryRequest, CPULimit, MemoryLimit          string
	UID, GID                                                  int64
	Images                                                    map[runtimecatalog.Runtime]string
	ProviderEnv                                               []string
	CodingProviders                                           []coderepository.CodingProvider
	CodingCredentialResolver                                  checkout.CredentialResolver
	StartupTimeout                                            time.Duration
	TerminationGraceSeconds                                   int64
	API                                                       API
}

type Executor struct {
	api       API
	config    Config
	providers map[string]checkout.Provider
	ctx       context.Context
	cancel    context.CancelFunc
}

func New(ctx context.Context, config Config) (*Executor, error) {
	if ctx == nil {
		return nil, errors.New("context is required")
	}
	if config.API == nil {
		api, err := inClusterAPI()
		if err != nil {
			return nil, err
		}
		config.API = api
	}
	return NewWithAPI(ctx, config, config.API)
}

func NewWithAPI(ctx context.Context, config Config, api API) (*Executor, error) {
	if ctx == nil || api == nil {
		return nil, errors.New("context and Kubernetes API are required")
	}
	for field, value := range map[string]string{"data root": config.DataRoot, "namespace": config.Namespace, "installation ID": config.InstallationID, "data PVC": config.DataPVC} {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("Kubernetes %s is required", field)
		}
	}
	if !dnsLabel(config.Namespace) {
		return nil, errors.New("Kubernetes namespace must be a DNS label")
	}
	if !dnsLabel(config.InstallationID) {
		return nil, errors.New("Kubernetes installation ID must be a DNS label")
	}
	if !dnsLabel(config.DataPVC) {
		return nil, errors.New("Kubernetes data PVC name must be a DNS label")
	}
	if config.ServiceAccount != "" && !dnsLabel(config.ServiceAccount) {
		return nil, errors.New("Kubernetes worker service account must be a DNS label")
	}
	root, err := filepath.Abs(config.DataRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve Kubernetes data root: %w", err)
	}
	if config.UID == 0 {
		config.UID = int64(os.Getuid())
	}
	if config.GID == 0 {
		config.GID = int64(os.Getgid())
	}
	if config.UID <= 0 || config.GID <= 0 {
		return nil, errors.New("Kubernetes worker uid and gid must be positive non-root values")
	}
	if config.StartupTimeout <= 0 {
		config.StartupTimeout = 2 * time.Minute
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
	if (config.CodexAuthSecret == "") != (config.CodexAuthSecretKey == "") {
		return nil, errors.New("Kubernetes Codex auth Secret name and key must be configured together")
	}
	if config.CodexAuthSecret != "" && !dnsLabel(config.CodexAuthSecret) {
		return nil, errors.New("Kubernetes Codex auth Secret name must be a DNS label")
	}
	if config.CodexAuthSecretKey != "" && !secretKey(config.CodexAuthSecretKey) {
		return nil, errors.New("Kubernetes Codex auth Secret key is invalid")
	}
	for _, secret := range config.ImagePullSecrets {
		if !dnsLabel(secret) {
			return nil, fmt.Errorf("Kubernetes image-pull Secret %q must be a DNS label", secret)
		}
	}
	if config.TerminationGraceSeconds <= 0 {
		config.TerminationGraceSeconds = 10
	}
	images := make(map[runtimecatalog.Runtime]string, len(runtimecatalog.Providers()))
	for _, provider := range runtimecatalog.Providers() {
		images[provider.Runtime()] = provider.DefaultImage()
	}
	for runtime, image := range config.Images {
		if !runtime.Available() {
			return nil, fmt.Errorf("unsupported runtime image %q", runtime)
		}
		if strings.TrimSpace(image) != "" {
			images[runtime] = strings.TrimSpace(image)
		}
	}
	config.DataRoot, config.Images = root, images
	if config.DataSubpath != "" && (filepath.IsAbs(config.DataSubpath) || filepath.Clean(config.DataSubpath) == ".." || strings.HasPrefix(filepath.Clean(config.DataSubpath), ".."+string(filepath.Separator))) {
		return nil, errors.New("Kubernetes PVC data subpath must be relative and contained")
	}
	providers := make(map[string]checkout.Provider, len(config.CodingProviders))
	for _, provider := range config.CodingProviders {
		if provider == nil || provider.ProviderName() == "" {
			return nil, errors.New("code repository provider name is required")
		}
		providers[provider.ProviderName()] = provider
	}
	workerCtx, cancel := context.WithCancel(context.Background())
	return &Executor{api: api, config: config, providers: providers, ctx: workerCtx, cancel: cancel}, nil
}

var _ orchestration.AgentctlStarter = (*Executor)(nil)
var _ orchestration.AgentProcessController = (*Executor)(nil)

func (e *Executor) StartAgentctl(ctx context.Context, run model.RunConfig) (model.Process, error) {
	if err := run.Validate(); err != nil {
		return model.Process{}, err
	}
	image := run.ExecutionImage
	if image == "" {
		image = e.config.Images[run.Runtime]
	}
	if image == "" {
		return model.Process{}, fmt.Errorf("no Kubernetes image configured for runtime %q", run.Runtime)
	}
	work, _, scope, err := e.taskDirectories(run)
	if err != nil {
		return model.Process{}, err
	}
	preparer := checkout.Preparer{Providers: e.providers, Credentials: e.config.CodingCredentialResolver}
	if err = preparer.Prepare(ctx, work, run.WorkspaceID, run.Repositories); err != nil {
		return model.Process{}, fmt.Errorf("prepare task repositories: %w", err)
	}
	labels := e.labels(run)
	name := resourceName(e.config.InstallationID, run.RunID)
	servicePath := e.resourcePath("services", name)
	service := map[string]any{"apiVersion": "v1", "kind": "Service", "metadata": map[string]any{"name": name, "namespace": e.config.Namespace, "labels": labels}, "spec": map[string]any{"type": "ClusterIP", "selector": map[string]string{"orkest.run": labels["orkest.run"]}, "ports": []any{map[string]any{"name": "agentctl", "port": agentctlPort, "targetPort": agentctlPort, "protocol": "TCP"}}}}
	if _, err = e.ensureResource(ctx, servicePath, service, labels); err != nil {
		return model.Process{ID: e.processID(name, "")}, fmt.Errorf("create agentctl Service: %w", err)
	}
	command := []string{"-listen", "0.0.0.0:8081", "-runtime", string(run.Runtime), "-work-dir", "/workspace", "-state-dir", "/agentctl-state"}
	if run.TaskSessionID != "" {
		command = append(command, "-run-id", run.RunID)
	}
	env := make([]any, 0, len(e.config.ProviderEnv)+1)
	secretValues := make(map[string]string)
	for _, key := range e.config.ProviderEnv {
		key = strings.TrimSpace(key)
		if key == "" || key == "ORKEST_CODEX_AUTH_FILE" {
			continue
		}
		if value, ok := os.LookupEnv(key); ok {
			secretValues[key] = value
			command = append(command, "-env", key)
		}
	}
	useCodexAuth := run.Runtime == runtimecatalog.Codex && e.config.CodexAuthSecret != ""
	if useCodexAuth {
		const authPath = "/agentctl-state/codex-auth/auth.json"
		env = append(env, map[string]any{"name": "ORKEST_CODEX_AUTH_FILE", "value": authPath})
		command = append(command, "-env", "ORKEST_CODEX_AUTH_FILE")
	}
	if len(secretValues) > 0 {
		secretName := name + "-env"
		secret := map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": secretName, "namespace": e.config.Namespace, "labels": labels}, "type": "Opaque", "stringData": secretValues}
		if _, err = e.ensureResource(ctx, e.resourcePath("secrets", secretName), secret, labels); err != nil {
			return model.Process{ID: e.processID(name, "")}, fmt.Errorf("create agentctl environment Secret: %w", err)
		}
		for key := range secretValues {
			env = append(env, map[string]any{"name": key, "valueFrom": map[string]any{"secretKeyRef": map[string]any{"name": secretName, "key": key}}})
		}
	}
	base := strings.Trim(filepath.ToSlash(filepath.Join("tasks", scope)), "/")
	if e.config.DataSubpath != "" {
		base = pathJoin(e.config.DataSubpath, base)
	}
	uid, gid := e.config.UID, e.config.GID
	security := map[string]any{"runAsNonRoot": true, "runAsUser": uid, "runAsGroup": gid, "allowPrivilegeEscalation": false, "readOnlyRootFilesystem": false, "capabilities": map[string]any{"drop": []string{"ALL"}}}
	pod := map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": name, "namespace": e.config.Namespace, "labels": labels}, "spec": map[string]any{"restartPolicy": "Never", "automountServiceAccountToken": false, "serviceAccountName": e.config.ServiceAccount, "terminationGracePeriodSeconds": e.config.TerminationGraceSeconds, "securityContext": map[string]any{"runAsNonRoot": true, "runAsUser": uid, "runAsGroup": gid}, "containers": []any{map[string]any{"name": "agentctl", "image": image, "args": command, "env": env, "ports": []any{map[string]any{"name": "agentctl", "containerPort": agentctlPort, "protocol": "TCP"}}, "readinessProbe": map[string]any{"httpGet": map[string]any{"path": "/health", "port": "agentctl"}, "periodSeconds": 2, "failureThreshold": 90}, "securityContext": security, "volumeMounts": []any{map[string]any{"name": "task-work", "mountPath": "/workspace", "subPath": base + "/work"}, map[string]any{"name": "task-state", "mountPath": "/agentctl-state", "subPath": base + "/agentctl-state"}}, "resources": map[string]any{"requests": map[string]string{"cpu": "250m", "memory": "512Mi"}, "limits": map[string]string{"cpu": "2", "memory": "4Gi"}}}}, "volumes": []any{map[string]any{"name": "task-work", "persistentVolumeClaim": map[string]any{"claimName": e.config.DataPVC, "readOnly": false}}, map[string]any{"name": "task-state", "persistentVolumeClaim": map[string]any{"claimName": e.config.DataPVC, "readOnly": false}}}}}
	if e.config.ServiceAccount == "" {
		delete(pod["spec"].(map[string]any), "serviceAccountName")
	}
	spec := pod["spec"].(map[string]any)
	containers := spec["containers"].([]any)
	containerSpec := containers[0].(map[string]any)
	containerSpec["resources"] = map[string]any{"requests": map[string]string{"cpu": e.config.CPURequest, "memory": e.config.MemoryRequest}, "limits": map[string]string{"cpu": e.config.CPULimit, "memory": e.config.MemoryLimit}}
	if len(e.config.ImagePullSecrets) > 0 {
		refs := make([]any, 0, len(e.config.ImagePullSecrets))
		for _, secret := range e.config.ImagePullSecrets {
			refs = append(refs, map[string]any{"name": secret})
		}
		spec["imagePullSecrets"] = refs
	}
	if useCodexAuth {
		volumes := spec["volumes"].([]any)
		volumes = append(volumes, map[string]any{"name": "codex-auth", "secret": map[string]any{"secretName": e.config.CodexAuthSecret, "items": []any{map[string]any{"key": e.config.CodexAuthSecretKey, "path": "auth.json"}}}})
		spec["volumes"] = volumes
		mounts := containerSpec["volumeMounts"].([]any)
		mounts = append(mounts, map[string]any{"name": "codex-auth", "mountPath": "/agentctl-state/codex-auth", "readOnly": true})
		containerSpec["volumeMounts"] = mounts
	}
	identity := e.processID(name, "")
	created, err := e.ensureResource(ctx, e.resourcePath("pods", name), pod, labels)
	if err != nil {
		return model.Process{ID: identity}, fmt.Errorf("create agentctl Pod: %w", err)
	}
	uidValue, _ := nestedString(created, "metadata", "uid")
	identity = e.processID(name, uidValue)
	waitCtx, cancel := context.WithTimeout(ctx, e.config.StartupTimeout)
	defer cancel()
	for {
		current, status, getErr := e.api.Do(waitCtx, http.MethodGet, e.resourcePath("pods", name), nil)
		if getErr != nil {
			return model.Process{ID: identity}, fmt.Errorf("inspect agentctl Pod: %w", getErr)
		}
		if status == 404 {
			return model.Process{ID: identity}, errors.New("agentctl Pod disappeared during startup")
		}
		phase, _ := nestedString(current, "status", "phase")
		if phase == "Running" && boolAt(current, "status", "containerStatuses", 0, "ready") {
			go e.forwardAgentctlLogs(name)
			return model.Process{ID: identity, Endpoint: fmt.Sprintf("http://%s.%s.svc:8081", name, e.config.Namespace)}, nil
		}
		if phase == "Failed" || phase == "Succeeded" {
			reason, _ := nestedString(current, "status", "reason")
			return model.Process{ID: identity}, fmt.Errorf("agentctl Pod exited during startup (%s)", reason)
		}
		select {
		case <-waitCtx.Done():
			return model.Process{ID: identity}, fmt.Errorf("wait for agentctl Pod readiness: %w", waitCtx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (e *Executor) TerminateAgentctl(ctx context.Context, id string) error {
	name, uid, err := e.parseID(id)
	if err != nil {
		return err
	}
	return e.deletePod(ctx, name, uid)
}
func (e *Executor) TerminateAgentctlByRunID(ctx context.Context, runID string) error {
	if strings.TrimSpace(runID) == "" {
		return errors.New("agent run identity is required")
	}
	return e.deletePod(ctx, resourceName(e.config.InstallationID, runID), "")
}
func (e *Executor) AgentctlRunning(ctx context.Context, id string) (bool, error) {
	name, uid, err := e.parseID(id)
	if err != nil {
		return false, err
	}
	return e.podRunning(ctx, name, uid)
}
func (e *Executor) AgentctlRunningByRunID(ctx context.Context, runID string) (bool, error) {
	if strings.TrimSpace(runID) == "" {
		return false, errors.New("agent run identity is required")
	}
	return e.podRunning(ctx, resourceName(e.config.InstallationID, runID), "")
}
func (e *Executor) podRunning(ctx context.Context, name, uid string) (bool, error) {
	pod, status, err := e.api.Do(ctx, http.MethodGet, e.resourcePath("pods", name), nil)
	if err != nil {
		return false, fmt.Errorf("inspect Kubernetes agentctl Pod: %w", err)
	}
	if status == 404 {
		return false, nil
	}
	if uid != "" && nestedStringValue(pod, "metadata", "uid") != uid {
		return false, errors.New("Kubernetes Pod identity changed")
	}
	phase, _ := nestedString(pod, "status", "phase")
	return phase != "Succeeded" && phase != "Failed", nil
}
func (e *Executor) deletePod(ctx context.Context, name, uid string) error {
	pod, status, err := e.api.Do(ctx, http.MethodGet, e.resourcePath("pods", name), nil)
	if err != nil {
		return fmt.Errorf("inspect Kubernetes agentctl Pod before termination: %w", err)
	}
	if status == 404 {
		return e.deleteService(ctx, name)
	}
	owner, _ := nestedString(pod, "metadata", "labels", "orkest.installation")
	if owner != e.config.InstallationID {
		return errors.New("refusing to terminate Pod not owned by this Orkest installation")
	}
	actual, _ := nestedString(pod, "metadata", "uid")
	if uid != "" && uid != actual {
		return errors.New("refusing to terminate a different Pod UID")
	}
	options := map[string]any{"apiVersion": "v1", "kind": "DeleteOptions", "gracePeriodSeconds": e.config.TerminationGraceSeconds}
	if actual != "" {
		options["preconditions"] = map[string]string{"uid": actual}
	}
	_, status, err = e.api.Do(ctx, http.MethodDelete, e.resourcePath("pods", name), options)
	if err != nil {
		return fmt.Errorf("delete Kubernetes agentctl Pod: %w", err)
	}
	if status != 404 && status >= 300 {
		return fmt.Errorf("delete Kubernetes agentctl Pod returned HTTP %d", status)
	}
	for {
		_, status, err = e.api.Do(ctx, http.MethodGet, e.resourcePath("pods", name), nil)
		if err != nil {
			return fmt.Errorf("confirm Kubernetes Pod termination: %w", err)
		}
		if status == 404 {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for Kubernetes Pod termination: %w", ctx.Err())
		case <-time.After(300 * time.Millisecond):
		}
	}
	return e.deleteService(ctx, name)
}
func (e *Executor) deleteService(ctx context.Context, name string) error {
	if err := e.deleteOwned(ctx, "secrets", name+"-env"); err != nil {
		return err
	}
	return e.deleteOwned(ctx, "services", name)
}
func (e *Executor) deleteOwned(ctx context.Context, resource, name string) error {
	value, status, err := e.api.Do(ctx, http.MethodGet, e.resourcePath(resource, name), nil)
	if err != nil {
		return fmt.Errorf("inspect Kubernetes %s before cleanup: %w", resource, err)
	}
	if status == 404 {
		return nil
	}
	if nestedStringValue(value, "metadata", "labels", "orkest.installation") != e.config.InstallationID {
		return fmt.Errorf("refusing to delete Kubernetes %s not owned by this Orkest installation", resource)
	}
	_, status, err = e.api.Do(ctx, http.MethodDelete, e.resourcePath(resource, name), map[string]any{"apiVersion": "v1", "kind": "DeleteOptions"})
	if err != nil {
		return fmt.Errorf("delete Kubernetes %s: %w", resource, err)
	}
	if status >= 300 && status != 404 {
		return fmt.Errorf("delete Kubernetes %s returned HTTP %d", resource, status)
	}
	return nil
}
func (e *Executor) Close() error { e.cancel(); return nil }

func (e *Executor) forwardAgentctlLogs(name string) {
	logs, ok := e.api.(logAPI)
	if !ok {
		return
	}
	stream, err := logs.PodLogs(e.ctx, e.resourcePath("pods", name)+"/log?container=agentctl&follow=true")
	if err != nil {
		return
	}
	defer stream.Close()
	agentctllogs.Forward(stream, "pod", name)
}

func (e *Executor) taskDirectories(run model.RunConfig) (string, string, string, error) {
	scopeBytes, err := json.Marshal([]string{run.OrganizationID, run.WorkspaceID, run.TaskID})
	if err != nil {
		return "", "", "", err
	}
	sum := sha256.Sum256(scopeBytes)
	scope := fmt.Sprintf("%x", sum)
	base := filepath.Join(e.config.DataRoot, "tasks", scope)
	work, state := filepath.Join(base, "work"), filepath.Join(base, "agentctl-state")
	for _, p := range []string{work, state} {
		if err = os.MkdirAll(p, 0700); err != nil {
			return "", "", "", fmt.Errorf("create Kubernetes shared task directory: %w", err)
		}
		if err = os.Chmod(p, 0700); err != nil {
			return "", "", "", fmt.Errorf("restrict shared task directory permissions: %w", err)
		}
	}
	return work, state, scope, nil
}
func (e *Executor) labels(run model.RunConfig) map[string]string {
	hash := func(s string) string { x := sha256.Sum256([]byte(s)); return fmt.Sprintf("%x", x)[:32] }
	image := run.ExecutionImage
	if image == "" {
		image = e.config.Images[run.Runtime]
	}
	return map[string]string{"app.kubernetes.io/managed-by": "orkest", "orkest.installation": e.config.InstallationID, "orkest.run": hash(run.RunID), "orkest.scope": hash(run.OrganizationID + "/" + run.WorkspaceID + "/" + run.TaskID), "orkest.runtime": string(run.Runtime), "orkest.image": hash(image)}
}
func (e *Executor) resourcePath(resource, name string) string {
	return "/api/v1/namespaces/" + e.config.Namespace + "/" + resource + "/" + name
}
func (e *Executor) processID(name, uid string) string {
	return e.config.Namespace + "/" + name + "/" + uid
}
func (e *Executor) parseID(id string) (string, string, error) {
	parts := strings.Split(id, "/")
	if len(parts) != 3 || parts[0] != e.config.Namespace || !dnsLabel(parts[1]) {
		return "", "", errors.New("invalid Kubernetes agentctl process identity")
	}
	return parts[1], parts[2], nil
}

func (e *Executor) ensureResource(ctx context.Context, path string, desired map[string]any, labels map[string]string) (map[string]any, error) {
	_, status, err := e.api.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		created, code, createErr := e.api.Do(ctx, http.MethodPost, strings.TrimSuffix(path, "/"+lastPart(path)), desired)
		if createErr != nil {
			// The API may have persisted a resource even when its response was lost,
			// or a concurrent launch may have won the create race. Reconcile by name.
			existing, readStatus, readErr := e.api.Do(ctx, http.MethodGet, path, nil)
			if readErr == nil && readStatus < 300 {
				if err := verifyLabels(existing, labels, lastPart(path)); err != nil {
					return nil, err
				}
				return existing, nil
			}
			return nil, createErr
		}
		if code >= 300 {
			return nil, fmt.Errorf("Kubernetes create returned HTTP %d", code)
		}
		return created, nil
	}
	if status >= 300 {
		return nil, fmt.Errorf("Kubernetes read returned HTTP %d", status)
	}
	existing, _, err := e.api.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if err := verifyLabels(existing, labels, lastPart(path)); err != nil {
		return nil, err
	}
	return existing, nil
}

func verifyLabels(resource map[string]any, labels map[string]string, name string) error {
	existingLabels, _ := nestedMap(resource, "metadata", "labels")
	for key, value := range labels {
		if existingLabels[key] != value {
			return fmt.Errorf("refusing to adopt Kubernetes resource %s with mismatched ownership label %s", name, key)
		}
	}
	return nil
}
func lastPart(s string) string { p := strings.Split(s, "/"); return p[len(p)-1] }

func dnsLabel(s string) bool {
	if len(s) < 1 || len(s) > 63 || !(s[0] >= 'a' && s[0] <= 'z' || s[0] >= '0' && s[0] <= '9') {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return s[len(s)-1] != '-'
}
func secretKey(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '.' || char == '_' || char == '-') {
			return false
		}
	}
	return true
}
func resourceName(installation, run string) string {
	x := sha256.Sum256([]byte(installation + "/" + run))
	return "orkest-" + installation[:min(8, len(installation))] + "-" + fmt.Sprintf("%x", x[:])[:32]
}
func pathJoin(a, b string) string { return strings.Trim(filepath.ToSlash(filepath.Join(a, b)), "/") }
func nested(v map[string]any, keys ...string) any {
	var cur any = v
	for _, key := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[key]
	}
	return cur
}
func nestedString(v map[string]any, keys ...string) (string, bool) {
	s, ok := nested(v, keys...).(string)
	return s, ok
}
func nestedStringValue(v map[string]any, keys ...string) string {
	s, _ := nestedString(v, keys...)
	return s
}
func nestedMap(v map[string]any, keys ...string) (map[string]any, bool) {
	m, ok := nested(v, keys...).(map[string]any)
	return m, ok
}
func boolAt(v map[string]any, keys ...any) bool {
	cur := any(v)
	for _, key := range keys {
		switch k := key.(type) {
		case string:
			m, ok := cur.(map[string]any)
			if !ok {
				return false
			}
			cur = m[k]
		case int:
			a, ok := cur.([]any)
			if !ok || k >= len(a) {
				return false
			}
			cur = a[k]
		}
	}
	b, _ := cur.(bool)
	return b
}

type restAPI struct {
	base, tokenPath string
	client          *http.Client
}

func inClusterAPI() (API, error) {
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	port := os.Getenv("KUBERNETES_SERVICE_PORT_HTTPS")
	if host == "" {
		return nil, errors.New("Kubernetes executor requires in-cluster service discovery or an injected API client")
	}
	if port == "" {
		port = "443"
	}
	const tokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	tokenBytes, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("read Kubernetes service account token: %w", err)
	}
	if strings.TrimSpace(string(tokenBytes)) == "" {
		return nil, errors.New("Kubernetes service account token is empty")
	}
	ca, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("read Kubernetes CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, errors.New("Kubernetes CA bundle is invalid")
	}
	return &restAPI{base: "https://" + host + ":" + port, tokenPath: tokenPath, client: &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}}, nil
}

func (a *restAPI) authorize(req *http.Request) error {
	token, err := os.ReadFile(a.tokenPath)
	if err != nil {
		return fmt.Errorf("read Kubernetes service account token: %w", err)
	}
	value := strings.TrimSpace(string(token))
	if value == "" {
		return errors.New("Kubernetes service account token is empty")
	}
	req.Header.Set("Authorization", "Bearer "+value)
	return nil
}

func (a *restAPI) Do(ctx context.Context, method, path string, body any) (map[string]any, int, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reader = strings.NewReader(string(data))
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, reader)
	if err != nil {
		return nil, 0, err
	}
	if err = a.authorize(req); err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := a.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	var value map[string]any
	if res.StatusCode != http.StatusNotFound {
		if err = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&value); err != nil && err != io.EOF {
			return nil, res.StatusCode, err
		}
	}
	if res.StatusCode >= 300 && res.StatusCode != 404 {
		return value, res.StatusCode, fmt.Errorf("Kubernetes API returned HTTP %d", res.StatusCode)
	}
	return value, res.StatusCode, nil
}

func (a *restAPI) PodLogs(ctx context.Context, path string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.base+path, nil)
	if err != nil {
		return nil, err
	}
	if err = a.authorize(req); err != nil {
		return nil, err
	}
	streamClient := &http.Client{Transport: a.client.Transport}
	response, err := streamClient.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 300 {
		response.Body.Close()
		return nil, fmt.Errorf("Kubernetes pod logs returned HTTP %d", response.StatusCode)
	}
	return response.Body, nil
}
