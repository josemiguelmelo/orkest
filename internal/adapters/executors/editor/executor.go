// Package editor contains shared task-editor discovery and launch policy.
package editor

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
)

const (
	manifestTarget       = "/workspace/task.code-workspace"
	taskTarget           = "/task"
	DefaultImage         = "orkest/task-editor:4.145.2"
	DefaultReadOnlyImage = "orkest/task-editor:4.145.2-ro"
)

type Config struct {
	Image, ReadOnlyImage string
	RunAsUser            uint32
	RunAsGroup           uint32
	CompatibilityKey     string
}

type EditorExecutor struct {
	engine  engine.InfrastructureEngine
	config  Config
	tokenMu sync.Mutex
	tokens  map[string]string
}

type editorStartError struct {
	stage string
	err   error
}

func (e editorStartError) Error() string          { return e.err.Error() }
func (e editorStartError) Unwrap() error          { return e.err }
func (e editorStartError) LifecycleStage() string { return "start/" + e.stage }

func atEditorStartStage(stage string, err error) error {
	if err == nil {
		return nil
	}
	return editorStartError{stage: stage, err: err}
}

var _ taskeditor.EditorExecutor = (*EditorExecutor)(nil)

func New(infrastructure engine.InfrastructureEngine, config Config) (*EditorExecutor, error) {
	if infrastructure == nil {
		return nil, errors.New("editor executor engine is required")
	}
	config.Image = strings.TrimSpace(config.Image)
	config.ReadOnlyImage = strings.TrimSpace(config.ReadOnlyImage)
	if config.Image == "" {
		config.Image = DefaultImage
	}
	if config.ReadOnlyImage == "" {
		config.ReadOnlyImage = DefaultReadOnlyImage
	}
	if config.Image == config.ReadOnlyImage {
		return nil, errors.New("distinct editor and read-only editor images are required")
	}
	if config.RunAsUser == 0 || config.RunAsGroup == 0 {
		return nil, errors.New("editor UID and GID must be non-root values")
	}
	if config.CompatibilityKey == "" {
		data, _ := json.Marshal(config)
		hash := sha256.Sum256(data)
		config.CompatibilityKey = hex.EncodeToString(hash[:])
	}
	return &EditorExecutor{engine: infrastructure, config: config, tokens: make(map[string]string)}, nil
}

func (e *EditorExecutor) Inspect(ctx context.Context, organizationID, workspaceID, taskID string, selected []taskeditor.Checkout) (taskeditor.Inspection, error) {
	storageID := taskstorage.Identity(organizationID, workspaceID, taskID)
	bindings, err := e.engine.ResolveStorage(ctx, engine.StorageRequest{Owner: storageID, Generation: "editor-inspection", Bindings: []engine.StorageBindingRequest{{Name: "work", StorageID: storageID, Subpath: "work", Durable: true}}})
	if errors.Is(err, os.ErrNotExist) {
		inspection := taskeditor.Inspection{CompatibilityKey: e.config.CompatibilityKey, Roots: []taskeditor.Root{}, Missing: missing(selected)}
		return inspection, nil
	}
	if err != nil {
		return taskeditor.Inspection{}, err
	}
	inspection := taskeditor.Inspection{CompatibilityKey: e.config.CompatibilityKey, Roots: []taskeditor.Root{}, Missing: []taskeditor.MissingCheckout{}}
	if len(bindings) != 1 || bindings[0].HostPath == "" {
		inspection.Missing = missing(selected)
		return inspection, nil
	}
	canonical, err := filepath.EvalSymlinks(bindings[0].HostPath)
	if errors.Is(err, os.ErrNotExist) {
		inspection.Missing = missing(selected)
		return inspection, nil
	}
	if err != nil {
		return taskeditor.Inspection{}, fmt.Errorf("resolve editor task workspace: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return taskeditor.Inspection{}, fmt.Errorf("inspect editor task workspace: %w", err)
	}
	if !info.IsDir() {
		return taskeditor.Inspection{}, errors.New("editor task workspace is not a directory")
	}
	inspection.Roots = append(inspection.Roots, taskeditor.Root{Name: "Task workspace", Path: taskTarget, StorageRef: canonical})
	return inspection, nil
}

func (e *EditorExecutor) StartEditor(ctx context.Context, spec taskeditor.Spec) (taskeditor.Instance, error) {
	instance := spec.Instance
	if len(instance.Generation) != 64 {
		return taskeditor.Instance{}, atEditorStartStage("validate_generation", errors.New("invalid editor generation"))
	}
	if _, err := hex.DecodeString(instance.Generation); err != nil {
		return taskeditor.Instance{}, atEditorStartStage("validate_generation", errors.New("invalid editor generation"))
	}
	if instance.UserID == "" || instance.Fingerprint == "" {
		return taskeditor.Instance{}, atEditorStartStage("validate_identity", errors.New("editor owner and fingerprint are required"))
	}
	if instance.Mode != taskeditor.ReadOnly && instance.Mode != taskeditor.ReadWrite {
		return taskeditor.Instance{}, atEditorStartStage("validate_mode", errors.New("invalid editor mode"))
	}
	if len(spec.Discovery.Roots) != 1 || spec.Discovery.Roots[0].Path != taskTarget || len(spec.Discovery.Manifest) == 0 {
		return taskeditor.Instance{}, atEditorStartStage("validate_discovery", taskeditor.ErrUnavailable)
	}
	storageID := taskstorage.Identity(instance.Scope.OrganizationID, instance.Scope.WorkspaceID, instance.Scope.TaskID)
	bindings, err := e.engine.ResolveStorage(ctx, engine.StorageRequest{Owner: storageID, Generation: instance.Generation, Bindings: []engine.StorageBindingRequest{{Name: "work", StorageID: storageID, Subpath: "work", Durable: true}}})
	if err != nil {
		return taskeditor.Instance{}, atEditorStartStage("resolve_storage", err)
	}
	if len(bindings) != 1 {
		return taskeditor.Instance{}, atEditorStartStage("validate_storage", engine.ErrInvalidSpec)
	}
	resolvedWork, resolveErr := filepath.EvalSymlinks(bindings[0].HostPath)
	if resolveErr != nil || resolvedWork != spec.Discovery.Roots[0].StorageRef {
		return taskeditor.Instance{}, atEditorStartStage("verify_workspace_identity", errors.New("editor task workspace changed after discovery"))
	}
	plan, token, planErr := e.infrastructurePlan(instance, storageID, spec.Discovery.Manifest)
	if planErr != nil {
		return taskeditor.Instance{}, atEditorStartStage("build_plan", planErr)
	}
	if err = e.engine.ValidatePlan(ctx, plan); err != nil {
		return taskeditor.Instance{}, atEditorStartStage("validate_plan", err)
	}
	if err = e.engine.ValidateStorageAccess(ctx, engine.StorageAccessRequest{StorageID: storageID, Subpath: "work"}); err != nil {
		return taskeditor.Instance{}, atEditorStartStage("validate_storage_access", fmt.Errorf("validate task storage admission for editor: %w", err))
	}
	group, startErr := e.engine.StartPlan(ctx, plan)
	instance.ID, err = engine.EncodeGroupRef(group.Ref)
	if err != nil {
		return instance, atEditorStartStage("encode_group_reference", err)
	}
	instance.Endpoint = group.Endpoints[plan.Metadata["endpoint"]]
	instance.GatewayToken = token
	instance.GatewayTokenRequired = plan.Metadata["gateway-token-required"] == "true"
	if instance.GatewayTokenRequired {
		e.tokenMu.Lock()
		e.tokens[instance.Generation] = token
		e.tokenMu.Unlock()
	}
	if startErr != nil {
		return instance, atEditorStartStage("start_plan", fmt.Errorf("start editor infrastructure plan: %w", startErr))
	}
	if instance.Endpoint == "" {
		return instance, atEditorStartStage("resolve_endpoint", engine.ErrUncertain)
	}
	if err = waitForEditorReady(ctx, instance.Endpoint, token); err != nil {
		return instance, atEditorStartStage("wait_for_readiness", err)
	}
	return instance, nil
}

func (e *EditorExecutor) infrastructurePlan(instance taskeditor.Instance, storageID string, manifest []byte) (engine.WorkloadPlan, string, error) {
	privateRouting := e.engine.Capabilities().PrivateServiceRouting
	token := ""
	if !privateRouting {
		if !e.engine.Capabilities().DeniedEgress || !e.engine.Capabilities().PlatformIngress {
			return engine.WorkloadPlan{}, "", engine.Unsupported("editor private routing or authenticated platform ingress")
		}
		tokenBytes := make([]byte, 32)
		if _, err := rand.Read(tokenBytes); err != nil {
			return engine.WorkloadPlan{}, "", fmt.Errorf("generate editor gateway credential: %w", err)
		}
		token = hex.EncodeToString(tokenBytes)
	}
	mode := string(instance.Mode)
	metadata := map[string]string{
		"executor": "task-editor", "organization": instance.Scope.OrganizationID,
		"workspace": instance.Scope.WorkspaceID, "task": instance.Scope.TaskID,
		"user": instance.UserID, "mode": mode, "fingerprint": instance.Fingerprint,
		"endpoint": "editor.http", "gateway-token-required": "false",
	}
	if !privateRouting {
		metadata["endpoint"] = "gateway"
		metadata["gateway-token-required"] = "true"
	}
	image := e.config.Image
	if instance.Mode == taskeditor.ReadOnly {
		image = e.config.ReadOnlyImage
	}
	security := engine.SecurityRequirements{ReadOnlyRoot: true, RunAsUser: e.config.RunAsUser, RunAsGroup: e.config.RunAsGroup, DropAllCapabilities: true, NoPrivilegeEscalation: true}
	members := []string{"editor"}
	if !privateRouting {
		members = append(members, "gateway")
	}
	plan := engine.WorkloadPlan{
		Group:    engine.GroupIdentity{Owner: storageID, Generation: instance.Generation},
		Metadata: metadata,
		Networks: []engine.NetworkPlan{{ID: "private", Members: members, DenyEgress: true}},
		Files: []engine.GeneratedFile{{
			Name: "task.code-workspace", WorkloadID: "editor", Target: manifestTarget,
			Contents: append([]byte(nil), manifest...), Mode: 0o444,
		}},
		Storage: []engine.StorageAttachment{{
			WorkloadID: "editor", Binding: "task-work", StorageID: storageID,
			Subpath: "work", Target: taskTarget, ReadOnly: instance.Mode == taskeditor.ReadOnly,
			Durable: true,
		}},
		Workloads: []engine.Workload{
			{
				ID: "editor", Mode: engine.ExecutionModeContainer, Artifact: engine.Artifact{ContainerImage: image},
				Environment:     map[string]string{"HOME": "/home/editor", "EDITOR_MODE": mode},
				Endpoints:       []engine.Endpoint{{Name: "http", Protocol: "http", ListenHost: "0.0.0.0", ListenPort: 8080, Exposure: engine.ExposurePrivate}},
				TemporaryMounts: []engine.TemporaryMount{{Target: "/tmp", SizeBytes: 256 << 20, Mode: 0o1777, NoSUID: true, NoDevice: true}, {Target: "/home/editor", SizeBytes: 512 << 20, Mode: 0o1777, NoSUID: true, NoDevice: true}},
				Security:        security, Limits: engine.ResourceLimits{MemoryBytes: 2 << 30, NanoCPUs: 2_000_000_000, ProcessCount: 256}, Restart: engine.RestartNever,
			},
		},
	}
	if !privateRouting {
		plan.Workloads = append(plan.Workloads, engine.Workload{
			ID: "gateway", Mode: engine.ExecutionModeContainer,
			Artifact:                engine.Artifact{ContainerImage: image, ContainerEntrypoint: []string{"/opt/code-server/lib/node", "/opt/editor/relay.cjs"}},
			Environment:             map[string]string{"ORKEST_EDITOR_RELAY_TOKEN": token},
			EnvironmentEndpointRefs: map[string]engine.EndpointReference{"ORKEST_EDITOR_RELAY_UPSTREAM": {Workload: "editor", Endpoint: "http"}},
			Endpoints:               []engine.Endpoint{{Name: "gateway", Protocol: "http", ListenHost: "0.0.0.0", ListenPort: 8081, Exposure: engine.ExposurePlatform}},
			Security:                security, Limits: engine.ResourceLimits{MemoryBytes: 128 << 20, NanoCPUs: 1_000_000_000, ProcessCount: 32}, Restart: engine.RestartNever,
		})
	}
	return plan, token, nil
}

func waitForEditorReady(ctx context.Context, endpoint, token string) error {
	readyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	probe := &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for {
		request, requestErr := http.NewRequestWithContext(readyCtx, http.MethodGet, endpoint+"/healthz", nil)
		if requestErr == nil {
			if token != "" {
				request.Header.Set("X-Orkest-Editor-Relay", token)
			}
			response, doErr := probe.Do(request)
			if doErr == nil {
				_ = response.Body.Close()
				if response.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
		select {
		case <-readyCtx.Done():
			return fmt.Errorf("editor gateway readiness: %w", readyCtx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (e *EditorExecutor) InspectEditor(ctx context.Context, scope taskeditor.Scope) (taskeditor.Instance, bool, error) {
	owner := taskstorage.Identity(scope.OrganizationID, scope.WorkspaceID, scope.TaskID)
	groups, err := e.engine.ListGroups(ctx, engine.Selector{GroupOwner: owner})
	if err != nil {
		return taskeditor.Instance{}, false, err
	}
	var found *engine.ResourceGroup
	for index := range groups {
		if groups[index].Metadata["executor"] != "task-editor" {
			continue
		}
		if groups[index].Metadata["organization"] != scope.OrganizationID || groups[index].Metadata["workspace"] != scope.WorkspaceID || groups[index].Metadata["task"] != scope.TaskID {
			return taskeditor.Instance{}, false, engine.ErrIdentityMismatch
		}
		if found != nil {
			return taskeditor.Instance{}, false, engine.ErrUncertain
		}
		found = &groups[index]
	}
	if found == nil {
		return taskeditor.Instance{}, false, nil
	}
	return e.infrastructureInstance(*found), true, nil
}

func (e *EditorExecutor) StopEditor(ctx context.Context, instance taskeditor.Instance) error {
	groupRef, err := engine.DecodeGroupRef(instance.ID)
	if err != nil {
		return err
	}
	owner := taskstorage.Identity(instance.Scope.OrganizationID, instance.Scope.WorkspaceID, instance.Scope.TaskID)
	if groupRef.Identity.Owner != owner || groupRef.Identity.Generation != instance.Generation {
		return engine.ErrIdentityMismatch
	}
	if err = e.engine.StopGroup(ctx, groupRef); err != nil {
		return err
	}
	if err = e.engine.CleanupGroup(ctx, groupRef); err != nil {
		return err
	}
	e.tokenMu.Lock()
	delete(e.tokens, instance.Generation)
	e.tokenMu.Unlock()
	return nil
}

func (e *EditorExecutor) ListEditors(ctx context.Context) ([]taskeditor.Instance, error) {
	groups, err := e.engine.ListGroups(ctx, engine.Selector{})
	if err != nil {
		return nil, err
	}
	instances := make([]taskeditor.Instance, 0, len(groups))
	for _, group := range groups {
		if group.Metadata["executor"] == "task-editor" {
			instances = append(instances, e.infrastructureInstance(group))
		}
	}
	return instances, nil
}

func (e *EditorExecutor) infrastructureInstance(group engine.ResourceGroup) taskeditor.Instance {
	ref := group.Ref
	id, _ := engine.EncodeGroupRef(ref)
	e.tokenMu.Lock()
	token := e.tokens[ref.Identity.Generation]
	e.tokenMu.Unlock()
	return taskeditor.Instance{
		ID: id, Scope: taskeditor.Scope{
			OrganizationID: group.Metadata["organization"], WorkspaceID: group.Metadata["workspace"], TaskID: group.Metadata["task"],
		},
		UserID: group.Metadata["user"], Generation: ref.Identity.Generation,
		Fingerprint: group.Metadata["fingerprint"], Mode: taskeditor.Mode(group.Metadata["mode"]),
		Endpoint: group.Endpoints[group.Metadata["endpoint"]], GatewayToken: token, GatewayTokenRequired: group.Metadata["gateway-token-required"] == "true",
	}
}

func missing(checkouts []taskeditor.Checkout) []taskeditor.MissingCheckout {
	result := make([]taskeditor.MissingCheckout, 0, len(checkouts))
	for _, checkout := range checkouts {
		result = append(result, taskeditor.MissingCheckout(checkout))
	}
	return result
}
