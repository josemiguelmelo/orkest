package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
)

type EngineConfig struct {
	DataRoot, DataSubpath, Namespace, InstallationID, DataPVC string
	ServiceAccount                                            string
	CredentialSources                                         map[string]engine.CredentialSource
	ImagePullSecrets                                          []string
	CPURequest, MemoryRequest, CPULimit, MemoryLimit          string
	UID, GID, TerminationGraceSeconds                         int64
	StartupTimeout                                            time.Duration
	Client                                                    API
	LogForwarder                                              engine.LogForwarder
}

// Engine runs agent workloads in an operator-managed namespace and PVC.
type Engine struct {
	config    EngineConfig
	client    API
	manifests *engine.ManifestStore
	ctx       context.Context
	cancel    context.CancelFunc
}

var _ engine.InfrastructureEngine = (*Engine)(nil)

func New(config EngineConfig) (*Engine, error) {
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
	for name, source := range config.CredentialSources {
		if !dnsLabel(name) || !dnsLabel(source.Secret) || !validSecretKey(source.Key) {
			return nil, errors.New("kubernetes credential source requires a DNS binding name and valid Secret/key")
		}
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
		return nil, errors.New("kubernetes engine workload UID and GID must be positive")
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
	ctx, cancel := context.WithCancel(context.Background())
	config.DataRoot = root
	manifests, err := engine.NewManifestStore(filepath.Join(root, "infrastructure-operations"))
	if err != nil {
		cancel()
		return nil, err
	}
	return &Engine{config: config, client: config.Client, manifests: manifests, ctx: ctx, cancel: cancel}, nil
}

func (e *Engine) Capabilities() engine.Capabilities {
	return engine.Capabilities{
		Modes:       []engine.ExecutionMode{engine.ExecutionModeContainer},
		DurableWork: true, DurableState: true, PlatformStorageSetup: true,
		ReadOnlyRoot: true, ReadOnlyMounts: true,
		CredentialBindings: len(e.config.CredentialSources) > 0,
	}
}

func (e *Engine) Close() error { e.cancel(); return nil }

func (e *Engine) resourcePath(resource, name string) string {
	base := "/api/v1/namespaces/" + e.config.Namespace + "/" + resource
	if name != "" {
		base += "/" + name
	}
	return base
}

func (e *Engine) processID(name, uid string) string {
	return e.config.Namespace + "/" + name + "/" + uid
}

func (e *Engine) parseID(id string) (string, string, error) {
	parts := strings.Split(id, "/")
	if len(parts) != 3 || parts[0] != e.config.Namespace || !dnsLabel(parts[1]) {
		return "", "", errors.New("invalid Kubernetes process identity")
	}
	return parts[1], parts[2], nil
}

func (e *Engine) ensureResource(ctx context.Context, requestPath string, desired map[string]any, labels map[string]string) (map[string]any, error) {
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

func (e *Engine) deleteOwned(ctx context.Context, resource, name string) error {
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
