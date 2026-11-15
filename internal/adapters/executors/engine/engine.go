// Package engine defines the infrastructure boundary shared by agent and editor
// executors. It contains only adapter-level values and no provider SDK types.
package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type ExecutionMode uint8

const (
	ExecutionModeUnknown ExecutionMode = iota
	ExecutionModeContainer
	ExecutionModeNative
)

type ResourceClass uint8

const (
	ResourceClassUnknown ResourceClass = iota
	ResourceClassAgent
	ResourceClassEditor
)

type ObservationState uint8

const (
	ObservationAbsent ObservationState = iota
	ObservationActive
	ObservationStopped
)

type WorkspaceIntent uint8

const (
	WorkspaceInspect WorkspaceIntent = iota
	WorkspaceEnsureForAgent
)

type Scope struct {
	OrganizationID string
	WorkspaceID    string
	TaskID         string
	SyntheticID    string
}

type ResourceKey struct {
	InstallationID string
	Scope          Scope
	Class          ResourceClass
	ExecutionID    string
	Qualifier      string
}

// ConcreteID is provider-owned and opaque. Empty means the stable key is the
// only available recovery handle.
type ResourceRef struct {
	Key        ResourceKey
	ConcreteID string
}

type Lookup struct {
	Ref ResourceRef
	Key *ResourceKey
}

type Selector struct {
	InstallationID string
	Class          ResourceClass
	Scope          *Scope
}

type Observation struct {
	State    ObservationState
	Resource Resource
}

type WorkspaceRequest struct {
	Scope    Scope
	Intent   WorkspaceIntent
	Bindings []WorkspaceBinding
	Required WorkspaceGuarantees
}

type WorkspaceBinding struct {
	Name string
	Path string
}

type WorkspaceGuarantees struct {
	DurableWork   bool
	DurableState  bool
	PlatformSetup bool
}

type Workspace struct {
	Identity             string
	Exists               bool
	PreparationDirectory string
	WorkPath             string
	StatePath            string
	CompatibilityID      string
}

type Artifact struct {
	ContainerImage      string
	ContainerEntrypoint []string
	ContainerCommand    []string
	ExecutablePath      string
}

type ArgumentKind uint8

const (
	ArgumentLiteral ArgumentKind = iota
	ArgumentEndpointAddress
)

type Argument struct {
	Kind     ArgumentKind
	Value    string
	Endpoint string
}

type Endpoint struct {
	Name          string
	Protocol      string
	ListenHost    string
	ListenPort    int
	ReadinessPath string
	Ingress       IngressMode
}

type IngressMode uint8

const (
	IngressDenied IngressMode = iota
	IngressWorkerControl
	IngressGatewayOnly
)

type Mount struct {
	SourceKind  MountSourceKind
	Source      string
	Target      string
	ReadOnly    bool
	BindingName string
}

type MountSourceKind uint8

const (
	MountSourceUnknown MountSourceKind = iota
	MountSourceWorkspace
	MountSourceHost
	MountSourceGeneratedFile
	MountSourceCredentialBinding
)

type GeneratedFile struct {
	Name     string
	Target   string
	Contents []byte
	Mode     uint32
}

type WorkspaceSharing struct {
	ProtectedAncestry bool
	ReadOnlyParents   []string
	WritableRoots     []string
}

type HostEnvironmentPolicy struct {
	InheritAllowlisted bool
	AllowedNames       []string
}

type CredentialBinding struct {
	Name     string
	Target   string
	ReadOnly bool
}

type SecurityRequirements struct {
	ReadOnlyRoot          bool
	RunAsUser             uint32
	RunAsGroup            uint32
	DropAllCapabilities   bool
	NoPrivilegeEscalation bool
}

type ResourceLimits struct {
	MemoryBytes  int64
	NanoCPUs     int64
	ProcessCount int64
}

type TemporaryMount struct {
	Target    string
	SizeBytes int64
	Mode      uint32
	NoSUID    bool
	NoDevice  bool
	NoExecute bool
}

type LaunchSpec struct {
	Key                ResourceKey
	Class              ResourceClass
	Mode               ExecutionMode
	Artifact           Artifact
	Arguments          []Argument
	Environment        map[string]string
	Workspace          Workspace
	Mounts             []Mount
	TemporaryMounts    []TemporaryMount
	GeneratedFiles     []GeneratedFile
	Sharing            WorkspaceSharing
	HostEnvironment    HostEnvironmentPolicy
	CredentialBindings []CredentialBinding
	Endpoints          []Endpoint
	Security           SecurityRequirements
	Limits             ResourceLimits
	Requirements       Requirements
	Restart            RestartPolicy
	Metadata           OwnershipMetadata
}

// ValidateSpec checks provider-neutral structure before an engine attempts to
// create resources. Capability and live-health validation remain separate.
func ValidateSpec(spec LaunchSpec) error {
	if spec.Class == ResourceClassUnknown || spec.Key.Class != spec.Class || spec.Key.ExecutionID == "" {
		return fmt.Errorf("%w: resource class and execution identity are required", ErrInvalidSpec)
	}
	switch spec.Mode {
	case ExecutionModeContainer:
		if spec.Artifact.ContainerImage == "" || spec.Artifact.ExecutablePath != "" {
			return fmt.Errorf("%w: container launch requires only a container image", ErrInvalidSpec)
		}
	case ExecutionModeNative:
		if spec.Artifact.ExecutablePath == "" || spec.Artifact.ContainerImage != "" || len(spec.Artifact.ContainerEntrypoint) > 0 || len(spec.Artifact.ContainerCommand) > 0 {
			return fmt.Errorf("%w: native launch requires only an executable path", ErrInvalidSpec)
		}
	default:
		return fmt.Errorf("%w: execution mode is required", ErrInvalidSpec)
	}
	endpoints := make(map[string]struct{}, len(spec.Endpoints))
	for _, endpoint := range spec.Endpoints {
		if endpoint.Name == "" || endpoint.Protocol == "" || endpoint.ListenPort < 0 || endpoint.ListenPort > 65535 {
			return fmt.Errorf("%w: endpoint name, protocol, and valid port are required", ErrInvalidSpec)
		}
		if _, exists := endpoints[endpoint.Name]; exists {
			return fmt.Errorf("%w: duplicate endpoint %q", ErrInvalidSpec, endpoint.Name)
		}
		endpoints[endpoint.Name] = struct{}{}
	}
	for _, argument := range spec.Arguments {
		switch argument.Kind {
		case ArgumentLiteral:
			if argument.Endpoint != "" {
				return fmt.Errorf("%w: literal argument cannot reference an endpoint", ErrInvalidSpec)
			}
		case ArgumentEndpointAddress:
			if _, exists := endpoints[argument.Endpoint]; !exists {
				return fmt.Errorf("%w: argument references unknown endpoint %q", ErrInvalidSpec, argument.Endpoint)
			}
		default:
			return fmt.Errorf("%w: unknown argument kind", ErrInvalidSpec)
		}
	}
	for _, file := range spec.GeneratedFiles {
		if file.Name == "" || file.Target == "" || file.Mode&^0o777 != 0 {
			return fmt.Errorf("%w: generated file requires a name, target, and permission mode", ErrInvalidSpec)
		}
	}
	for _, mount := range spec.Mounts {
		if mount.SourceKind == MountSourceUnknown || mount.Source == "" || mount.Target == "" {
			return fmt.Errorf("%w: mount source kind, source, and target are required", ErrInvalidSpec)
		}
	}
	for _, mount := range spec.TemporaryMounts {
		if mount.Target == "" || !strings.HasPrefix(mount.Target, "/") || mount.SizeBytes < 0 || mount.Mode&^0o7777 != 0 {
			return fmt.Errorf("%w: temporary mount requires an absolute target and valid size/mode", ErrInvalidSpec)
		}
	}
	return nil
}

type RestartPolicy uint8

const (
	RestartNever RestartPolicy = iota
	RestartOnFailure
)

type Requirements struct {
	Mode                        ExecutionMode
	DurableWork                 bool
	DurableState                bool
	ReadOnlyRoot                bool
	ReadOnlyMounts              bool
	TemporaryMounts             bool
	ProtectedWorkspaceSharing   bool
	GatewayOnlyIngress          bool
	DeniedEgress                bool
	PrivateHTTPWebSocketRouting bool
	ConfirmedProcessTreeStop    bool
	ExplicitHostBinds           bool
	CredentialBindings          bool
	RestartModes                bool
}

type Capabilities struct {
	Modes                       []ExecutionMode
	DurableWork                 bool
	DurableState                bool
	PlatformWorkspaceSetup      bool
	ReadOnlyRoot                bool
	ReadOnlyMounts              bool
	TemporaryMounts             bool
	ProtectedWorkspaceSharing   bool
	GatewayOnlyIngress          bool
	DeniedEgress                bool
	PrivateHTTPWebSocketRouting bool
	ConfirmedProcessTreeStop    bool
	ExplicitHostBinds           bool
	CredentialBindings          bool
	RestartModes                bool
}

// MissingCapabilities reports each requested guarantee the engine does not
// advertise. Runtime health checks remain the responsibility of Validate.
func (c Capabilities) Missing(r Requirements) []string {
	var missing []string
	if r.Mode != ExecutionModeUnknown && !containsMode(c.Modes, r.Mode) {
		missing = append(missing, "requested execution mode")
	}
	checks := []struct {
		name      string
		required  bool
		supported bool
	}{
		{"durable work storage", r.DurableWork, c.DurableWork},
		{"durable state storage", r.DurableState, c.DurableState},
		{"read-only root filesystem", r.ReadOnlyRoot, c.ReadOnlyRoot},
		{"read-only mounts", r.ReadOnlyMounts, c.ReadOnlyMounts},
		{"temporary mounts", r.TemporaryMounts, c.TemporaryMounts},
		{"protected workspace sharing", r.ProtectedWorkspaceSharing, c.ProtectedWorkspaceSharing},
		{"gateway-only ingress", r.GatewayOnlyIngress, c.GatewayOnlyIngress},
		{"denied egress", r.DeniedEgress, c.DeniedEgress},
		{"private HTTP/WebSocket routing", r.PrivateHTTPWebSocketRouting, c.PrivateHTTPWebSocketRouting},
		{"confirmed process-tree stop", r.ConfirmedProcessTreeStop, c.ConfirmedProcessTreeStop},
		{"explicit host binds", r.ExplicitHostBinds, c.ExplicitHostBinds},
		{"credential bindings", r.CredentialBindings, c.CredentialBindings},
		{"restart modes", r.RestartModes, c.RestartModes},
	}
	for _, check := range checks {
		if check.required && !check.supported {
			missing = append(missing, check.name)
		}
	}
	return missing
}

func ValidateCapabilities(capabilities Capabilities, requirements Requirements) error {
	missing := capabilities.Missing(requirements)
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrUnsupported, strings.Join(missing, ", "))
}

func ValidateWorkspaceGuarantees(capabilities Capabilities, requirements WorkspaceGuarantees) error {
	var missing []string
	if requirements.DurableWork && !capabilities.DurableWork {
		missing = append(missing, "durable work storage")
	}
	if requirements.DurableState && !capabilities.DurableState {
		missing = append(missing, "durable state storage")
	}
	if requirements.PlatformSetup && !capabilities.PlatformWorkspaceSetup {
		missing = append(missing, "platform-accessible workspace setup")
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrUnsupported, strings.Join(missing, ", "))
}

func containsMode(modes []ExecutionMode, mode ExecutionMode) bool {
	for _, candidate := range modes {
		if candidate == mode {
			return true
		}
	}
	return false
}

type ResourceState uint8

const (
	ResourceUnknown ResourceState = iota
	ResourceStarting
	ResourceRunning
	ResourceStopping
	ResourceStopped
)

type OwnershipMetadata struct {
	OwnerID     string
	Generation  uint64
	Mode        string
	Fingerprint string
	Qualifier   string
}

type Resource struct {
	Ref      ResourceRef
	State    ResourceState
	Endpoint string
	Access   string
	Metadata OwnershipMetadata
}

type ExecutorEngine interface {
	Capabilities() Capabilities
	Validate(context.Context, Requirements) error
	ResolveWorkspace(context.Context, WorkspaceRequest) (Workspace, error)
	Start(context.Context, LaunchSpec) (Resource, error)
	Inspect(context.Context, Lookup) (Observation, error)
	List(context.Context, Selector) ([]Resource, error)
	Stop(context.Context, ResourceRef) error
	Cleanup(context.Context, ResourceRef) error
	Close() error
}

var (
	ErrUnsupported      = errors.New("executor requirement is unsupported")
	ErrInvalidSpec      = errors.New("invalid executor launch specification")
	ErrIdentityMismatch = errors.New("executor resource identity mismatch")
	ErrUncertain        = errors.New("executor operation outcome is uncertain")
)

func Unsupported(requirement string) error {
	return fmt.Errorf("%w: %s", ErrUnsupported, requirement)
}
