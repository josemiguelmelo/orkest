// Package editor contains shared task-editor discovery and launch policy.
package editor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
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
	engine engine.ExecutorEngine
	config Config
}

var _ taskeditor.EditorExecutor = (*EditorExecutor)(nil)

func New(executorEngine engine.ExecutorEngine, config Config) (*EditorExecutor, error) {
	if executorEngine == nil {
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
	return &EditorExecutor{engine: executorEngine, config: config}, nil
}

func (e *EditorExecutor) Inspect(ctx context.Context, organizationID, workspaceID, taskID string, selected []taskeditor.Checkout) (taskeditor.Inspection, error) {
	scope := engine.Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, TaskID: taskID}
	workspace, err := e.engine.ResolveWorkspace(ctx, engine.WorkspaceRequest{Scope: scope, Intent: engine.WorkspaceInspect, Required: engine.WorkspaceGuarantees{DurableWork: true}})
	if err != nil {
		return taskeditor.Inspection{}, err
	}
	inspection := taskeditor.Inspection{CompatibilityKey: e.config.CompatibilityKey, Roots: []taskeditor.Root{}, Missing: []taskeditor.MissingCheckout{}}
	if !workspace.Exists {
		inspection.Missing = missing(selected)
		return inspection, nil
	}
	canonical, err := filepath.EvalSymlinks(workspace.WorkPath)
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
		return taskeditor.Instance{}, errors.New("invalid editor generation")
	}
	if _, err := hex.DecodeString(instance.Generation); err != nil {
		return taskeditor.Instance{}, errors.New("invalid editor generation")
	}
	if instance.UserID == "" || instance.Fingerprint == "" {
		return taskeditor.Instance{}, errors.New("editor owner and fingerprint are required")
	}
	if instance.Mode != taskeditor.ReadOnly && instance.Mode != taskeditor.ReadWrite {
		return taskeditor.Instance{}, errors.New("invalid editor mode")
	}
	if len(spec.Discovery.Roots) != 1 || spec.Discovery.Roots[0].Path != taskTarget || len(spec.Discovery.Manifest) == 0 {
		return taskeditor.Instance{}, taskeditor.ErrUnavailable
	}
	mode := e.config.Image
	readOnly := instance.Mode == taskeditor.ReadOnly
	if readOnly {
		mode = e.config.ReadOnlyImage
	}
	scope := engine.Scope{OrganizationID: instance.Scope.OrganizationID, WorkspaceID: instance.Scope.WorkspaceID, TaskID: instance.Scope.TaskID}
	requirements := engine.Requirements{
		Mode: engine.ExecutionModeContainer, DurableWork: true, ReadOnlyRoot: true, ReadOnlyMounts: true,
		TemporaryMounts: true, ProtectedWorkspaceSharing: true, GatewayOnlyIngress: true,
		DeniedEgress: true, PrivateHTTPWebSocketRouting: true, ConfirmedProcessTreeStop: true,
	}
	if err := e.engine.Validate(ctx, requirements); err != nil {
		return taskeditor.Instance{}, err
	}
	workspace, err := e.engine.ResolveWorkspace(ctx, engine.WorkspaceRequest{Scope: scope, Intent: engine.WorkspaceInspect, Required: engine.WorkspaceGuarantees{DurableWork: true}})
	if err != nil {
		return taskeditor.Instance{}, err
	}
	resolvedWork, resolveErr := filepath.EvalSymlinks(workspace.WorkPath)
	if resolveErr != nil || !workspace.Exists || resolvedWork != spec.Discovery.Roots[0].StorageRef {
		return taskeditor.Instance{}, errors.New("editor task workspace changed after discovery")
	}
	workspace.WorkPath = spec.Discovery.Roots[0].StorageRef
	key := engine.ResourceKey{Scope: scope, Class: engine.ResourceClassEditor, ExecutionID: instance.Generation, Qualifier: "task-editor"}
	sharing := engine.WorkspaceSharing{ProtectedAncestry: true}
	launch := engine.LaunchSpec{
		Key: key, Class: engine.ResourceClassEditor, Mode: engine.ExecutionModeContainer,
		Artifact: engine.Artifact{ContainerImage: mode}, Workspace: workspace,
		Mounts:         []engine.Mount{{SourceKind: engine.MountSourceWorkspace, Source: workspace.WorkPath, Target: taskTarget, ReadOnly: readOnly}},
		GeneratedFiles: []engine.GeneratedFile{{Name: "task.code-workspace", Target: manifestTarget, Contents: append([]byte(nil), spec.Discovery.Manifest...), Mode: 0o444}},
		TemporaryMounts: []engine.TemporaryMount{
			{Target: "/tmp", SizeBytes: 256 << 20, Mode: 0o1777, NoSUID: true, NoDevice: true},
			{Target: "/home/editor", SizeBytes: 512 << 20, Mode: 0o1777, NoSUID: true, NoDevice: true},
		},
		Sharing:      sharing,
		Endpoints:    []engine.Endpoint{{Name: "editor", Protocol: "http", ListenHost: "0.0.0.0", ListenPort: 8080, ReadinessPath: "/healthz", Ingress: engine.IngressGatewayOnly}},
		Security:     engine.SecurityRequirements{ReadOnlyRoot: true, RunAsUser: e.config.RunAsUser, RunAsGroup: e.config.RunAsGroup, DropAllCapabilities: true, NoPrivilegeEscalation: true},
		Limits:       engine.ResourceLimits{MemoryBytes: 2 << 30, NanoCPUs: 2_000_000_000, ProcessCount: 256},
		Requirements: requirements,
		Metadata:     engine.OwnershipMetadata{OwnerID: instance.UserID, Mode: string(instance.Mode), Fingerprint: instance.Fingerprint},
		Restart:      engine.RestartNever,
	}
	resource, err := e.engine.Start(ctx, launch)
	instance.ID, instance.Endpoint, instance.GatewayToken = resource.Ref.ConcreteID, resource.Endpoint, resource.Access
	if err != nil {
		return instance, err
	}
	return instance, nil
}

func (e *EditorExecutor) InspectEditor(ctx context.Context, scope taskeditor.Scope) (taskeditor.Instance, bool, error) {
	engineScope := engine.Scope{OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, TaskID: scope.TaskID}
	resources, err := e.engine.List(ctx, engine.Selector{Class: engine.ResourceClassEditor, Scope: &engineScope})
	if err != nil {
		return taskeditor.Instance{}, false, err
	}
	if len(resources) == 0 {
		return taskeditor.Instance{}, false, nil
	}
	if len(resources) != 1 {
		return taskeditor.Instance{}, false, engine.ErrUncertain
	}
	return e.instance(resources[0]), true, nil
}

func (e *EditorExecutor) StopEditor(ctx context.Context, instance taskeditor.Instance) error {
	ref := engine.ResourceRef{Key: engine.ResourceKey{Scope: engine.Scope{OrganizationID: instance.Scope.OrganizationID, WorkspaceID: instance.Scope.WorkspaceID, TaskID: instance.Scope.TaskID}, Class: engine.ResourceClassEditor, ExecutionID: instance.Generation, Qualifier: "task-editor"}, ConcreteID: instance.ID}
	if err := e.engine.Stop(ctx, ref); err != nil {
		return err
	}
	return e.engine.Cleanup(ctx, ref)
}

func (e *EditorExecutor) ListEditors(ctx context.Context) ([]taskeditor.Instance, error) {
	resources, err := e.engine.List(ctx, engine.Selector{Class: engine.ResourceClassEditor})
	if err != nil {
		return nil, err
	}
	instances := make([]taskeditor.Instance, 0, len(resources))
	for _, resource := range resources {
		instances = append(instances, e.instance(resource))
	}
	return instances, nil
}

func (e *EditorExecutor) instance(resource engine.Resource) taskeditor.Instance {
	return taskeditor.Instance{
		ID: resource.Ref.ConcreteID, Scope: taskeditor.Scope{OrganizationID: resource.Ref.Key.Scope.OrganizationID, WorkspaceID: resource.Ref.Key.Scope.WorkspaceID, TaskID: resource.Ref.Key.Scope.TaskID},
		UserID: resource.Metadata.OwnerID, Generation: resource.Ref.Key.ExecutionID, Fingerprint: resource.Metadata.Fingerprint,
		Mode: taskeditor.Mode(resource.Metadata.Mode), Endpoint: resource.Endpoint, GatewayToken: resource.Access,
	}
}

func missing(checkouts []taskeditor.Checkout) []taskeditor.MissingCheckout {
	result := make([]taskeditor.MissingCheckout, 0, len(checkouts))
	for _, checkout := range checkouts {
		result = append(result, taskeditor.MissingCheckout(checkout))
	}
	return result
}
