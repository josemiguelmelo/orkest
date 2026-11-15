// Package engine defines provider-neutral workload and storage contracts.
// Concrete implementations translate plans to provider resources.
package engine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

type ExecutionMode uint8

const (
	ExecutionModeUnknown ExecutionMode = iota
	ExecutionModeContainer
	ExecutionModeNative
)

type ResourceKey struct {
	InstallationID string
	ResourceID     string
	ComponentID    string
}

// ConcreteID is provider-owned and opaque. Empty means the stable key is the
// only available recovery handle.
type ResourceRef struct {
	Key        ResourceKey
	ConcreteID string
}

type Selector struct {
	InstallationID string
	GroupOwner     string
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
	Workload string
	Endpoint string
}

type EndpointReference struct {
	Workload string
	Endpoint string
}

type Endpoint struct {
	Name          string
	Protocol      string
	ListenHost    string
	ListenPort    int
	ReadinessPath string
	Exposure      Exposure
}

type Exposure uint8

const (
	ExposurePrivate Exposure = iota
	ExposurePlatform
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
	MountSourceStorage
	MountSourceHost
	MountSourceGeneratedFile
	MountSourceCredentialBinding
)

type GeneratedFile struct {
	Name       string
	WorkloadID string
	Target     string
	Contents   []byte
	Mode       uint32
}

type CredentialBinding struct {
	Name            string
	Target          string
	ReadOnly        bool
	EnvironmentName string
}

// CredentialSource is infrastructure-specific material resolved by composition.
// The binding name is opaque to an engine and the secret value is never included
// in resource metadata.
type CredentialSource struct {
	FilePath string
	Secret   string
	Key      string
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

// WorkloadPlan describes infrastructure without naming the application that
// produced it. A plan may contain multiple related workloads and infrastructure
// objects; GroupIdentity is the stable reconciliation boundary.
type WorkloadPlan struct {
	Group     GroupIdentity
	Metadata  map[string]string
	Workloads []Workload
	Networks  []NetworkPlan
	Files     []GeneratedFile
	Storage   []StorageAttachment
	Probes    []Probe
}

type GroupIdentity struct {
	InstallationID string
	Owner          string
	Generation     string
}

type Workload struct {
	ID                      string
	Mode                    ExecutionMode
	Artifact                Artifact
	Endpoints               []Endpoint
	Arguments               []Argument
	Environment             map[string]string
	EnvironmentEndpointRefs map[string]EndpointReference
	WorkingPath             string
	Mounts                  []Mount
	TemporaryMounts         []TemporaryMount
	Credentials             []CredentialBinding
	Security                SecurityRequirements
	Limits                  ResourceLimits
	Restart                 RestartPolicy
}

type NetworkPlan struct {
	ID         string
	Members    []string
	Ingress    []Endpoint
	DenyEgress bool
	// Existing names a pre-created provider network that the engine may join.
	// Empty means the engine creates and owns an isolated network.
	Existing string
}

type StorageAttachment struct {
	WorkloadID string
	Binding    string
	StorageID  string
	Subpath    string
	Target     string
	ReadOnly   bool
	Create     bool
	Durable    bool
	PlatformIO bool
}

type Probe struct {
	WorkloadID string
	Endpoint   string
	Path       string
	StatusCode int
}

// ValidateWorkloadPlan checks neutral plan structure before provider
// translation.
func ValidateWorkloadPlan(plan WorkloadPlan) error {
	if plan.Group.Owner == "" || plan.Group.Generation == "" || len(plan.Workloads) == 0 {
		return fmt.Errorf("%w: group identity and at least one workload are required", ErrInvalidSpec)
	}
	ids := make(map[string]struct{}, len(plan.Workloads))
	endpointIDs := make(map[string]map[string]struct{}, len(plan.Workloads))
	mountTargets := make(map[string]map[string]struct{}, len(plan.Workloads))
	for _, workload := range plan.Workloads {
		if workload.ID == "" {
			return fmt.Errorf("%w: workload identity is required", ErrInvalidSpec)
		}
		if _, exists := ids[workload.ID]; exists {
			return fmt.Errorf("%w: duplicate workload %q", ErrInvalidSpec, workload.ID)
		}
		ids[workload.ID] = struct{}{}
		endpointIDs[workload.ID] = make(map[string]struct{}, len(workload.Endpoints))
		switch workload.Mode {
		case ExecutionModeContainer:
			if workload.Artifact.ContainerImage == "" || workload.Artifact.ExecutablePath != "" {
				return fmt.Errorf("%w: container workload %q requires only an image", ErrInvalidSpec, workload.ID)
			}
		case ExecutionModeNative:
			if workload.Artifact.ExecutablePath == "" || workload.Artifact.ContainerImage != "" || len(workload.Artifact.ContainerEntrypoint) > 0 || len(workload.Artifact.ContainerCommand) > 0 {
				return fmt.Errorf("%w: native workload %q requires only an executable", ErrInvalidSpec, workload.ID)
			}
		default:
			return fmt.Errorf("%w: workload %q has no execution mode", ErrInvalidSpec, workload.ID)
		}
		if workload.Restart > RestartOnFailure {
			return fmt.Errorf("%w: workload %q has an unknown restart policy", ErrInvalidSpec, workload.ID)
		}
		endpoints := make(map[string]struct{}, len(workload.Endpoints))
		for _, endpoint := range workload.Endpoints {
			if endpoint.Name == "" || endpoint.Protocol == "" || endpoint.ListenPort < 0 || endpoint.ListenPort > 65535 {
				return fmt.Errorf("%w: workload %q has an invalid endpoint", ErrInvalidSpec, workload.ID)
			}
			if _, exists := endpoints[endpoint.Name]; exists {
				return fmt.Errorf("%w: workload %q repeats endpoint %q", ErrInvalidSpec, workload.ID, endpoint.Name)
			}
			endpoints[endpoint.Name] = struct{}{}
		}
		endpointIDs[workload.ID] = endpoints
		mountTargets[workload.ID] = make(map[string]struct{}, len(workload.Mounts)+len(workload.TemporaryMounts)+len(workload.Credentials))
		for _, mount := range workload.Mounts {
			if mount.SourceKind == MountSourceUnknown || mount.Source == "" || mount.Target == "" || !strings.HasPrefix(mount.Target, "/") || filepathClean(mount.Target) == "/" {
				return fmt.Errorf("%w: workload %q has an invalid mount", ErrInvalidSpec, workload.ID)
			}
			if err := addPlanMountTarget(mountTargets[workload.ID], mount.Target); err != nil {
				return fmt.Errorf("%w: workload %q mount %q conflicts with another attachment", ErrInvalidSpec, workload.ID, mount.Target)
			}
		}
		for _, mount := range workload.TemporaryMounts {
			if mount.Target == "" || !strings.HasPrefix(mount.Target, "/") || filepathClean(mount.Target) == "/" || mount.SizeBytes < 0 || mount.Mode&^0o7777 != 0 {
				return fmt.Errorf("%w: workload %q has an invalid temporary mount", ErrInvalidSpec, workload.ID)
			}
			if err := addPlanMountTarget(mountTargets[workload.ID], mount.Target); err != nil {
				return fmt.Errorf("%w: workload %q mount %q conflicts with another attachment", ErrInvalidSpec, workload.ID, mount.Target)
			}
		}
		credentialNames := make(map[string]struct{}, len(workload.Credentials))
		for _, credential := range workload.Credentials {
			if credential.Name == "" || credential.Target == "" || !strings.HasPrefix(credential.Target, "/") || filepathClean(credential.Target) == "/" {
				return fmt.Errorf("%w: workload %q has an invalid credential binding", ErrInvalidSpec, workload.ID)
			}
			if _, exists := credentialNames[credential.Name]; exists {
				return fmt.Errorf("%w: workload %q repeats credential binding %q", ErrInvalidSpec, workload.ID, credential.Name)
			}
			credentialNames[credential.Name] = struct{}{}
			if err := addPlanMountTarget(mountTargets[workload.ID], credential.Target); err != nil {
				return fmt.Errorf("%w: workload %q credential target %q conflicts with another attachment", ErrInvalidSpec, workload.ID, credential.Target)
			}
		}
	}
	for _, workload := range plan.Workloads {
		for name, reference := range workload.EnvironmentEndpointRefs {
			if strings.TrimSpace(name) == "" || strings.Contains(name, "=") || name == "" || strings.TrimSpace(reference.Endpoint) == "" {
				return fmt.Errorf("%w: workload %q has an invalid endpoint environment binding", ErrInvalidSpec, workload.ID)
			}
			if _, exists := workload.Environment[name]; exists {
				return fmt.Errorf("%w: workload %q repeats environment variable %q", ErrInvalidSpec, workload.ID, name)
			}
			targetWorkload := reference.Workload
			if targetWorkload == "" {
				targetWorkload = workload.ID
			}
			if _, exists := endpointIDs[targetWorkload][reference.Endpoint]; !exists {
				return fmt.Errorf("%w: workload %q environment references unknown endpoint %q on workload %q", ErrInvalidSpec, workload.ID, reference.Endpoint, targetWorkload)
			}
		}
		for _, argument := range workload.Arguments {
			switch argument.Kind {
			case ArgumentLiteral:
				if argument.Endpoint != "" || argument.Workload != "" {
					return fmt.Errorf("%w: literal argument cannot reference an endpoint", ErrInvalidSpec)
				}
			case ArgumentEndpointAddress:
				targetWorkload := argument.Workload
				if targetWorkload == "" {
					targetWorkload = workload.ID
				}
				if _, exists := endpointIDs[targetWorkload][argument.Endpoint]; !exists {
					return fmt.Errorf("%w: workload %q argument references unknown endpoint %q on workload %q", ErrInvalidSpec, workload.ID, argument.Endpoint, targetWorkload)
				}
			default:
				return fmt.Errorf("%w: workload %q has an unknown argument kind", ErrInvalidSpec, workload.ID)
			}
		}
	}
	networkIDs := make(map[string]struct{}, len(plan.Networks))
	for _, network := range plan.Networks {
		if network.ID == "" || network.Existing != "" && network.DenyEgress {
			return fmt.Errorf("%w: network identity is required", ErrInvalidSpec)
		}
		if _, exists := networkIDs[network.ID]; exists {
			return fmt.Errorf("%w: duplicate network %q", ErrInvalidSpec, network.ID)
		}
		networkIDs[network.ID] = struct{}{}
		if len(network.Members) == 0 {
			return fmt.Errorf("%w: network %q requires at least one member", ErrInvalidSpec, network.ID)
		}
		members := make(map[string]struct{}, len(network.Members))
		for _, member := range network.Members {
			if _, exists := ids[member]; !exists {
				return fmt.Errorf("%w: network references unknown workload %q", ErrInvalidSpec, member)
			}
			if _, exists := members[member]; exists {
				return fmt.Errorf("%w: network repeats workload %q", ErrInvalidSpec, member)
			}
			members[member] = struct{}{}
		}
		for _, endpoint := range network.Ingress {
			if endpoint.Name == "" || endpoint.Protocol == "" || endpoint.ListenPort < 1 || endpoint.ListenPort > 65535 {
				return fmt.Errorf("%w: network %q has an invalid ingress endpoint", ErrInvalidSpec, network.ID)
			}
		}
	}
	for _, probe := range plan.Probes {
		if _, exists := ids[probe.WorkloadID]; !exists || probe.Endpoint == "" || !strings.HasPrefix(probe.Path, "/") || probe.StatusCode < 100 || probe.StatusCode > 599 {
			return fmt.Errorf("%w: readiness probe must reference a workload, endpoint, absolute path and valid status", ErrInvalidSpec)
		}
		if _, exists := endpointIDs[probe.WorkloadID][probe.Endpoint]; !exists {
			return fmt.Errorf("%w: readiness probe references unknown endpoint %q on workload %q", ErrInvalidSpec, probe.Endpoint, probe.WorkloadID)
		}
	}
	fileNames := make(map[string]struct{}, len(plan.Files))
	fileTargets := make(map[string]struct{}, len(plan.Files))
	for _, file := range plan.Files {
		if file.Name == "" || file.Target == "" || !strings.HasPrefix(file.Target, "/") || filepathClean(file.Target) == "/" || file.Mode&^0o777 != 0 {
			return fmt.Errorf("%w: generated file requires a name, absolute non-root target and valid permission mode", ErrInvalidSpec)
		}
		if _, exists := fileNames[file.Name]; exists {
			return fmt.Errorf("%w: duplicate generated file %q", ErrInvalidSpec, file.Name)
		}
		if _, exists := fileTargets[file.Target]; exists {
			return fmt.Errorf("%w: duplicate generated file target %q", ErrInvalidSpec, file.Target)
		}
		if file.WorkloadID != "" {
			if _, exists := ids[file.WorkloadID]; !exists {
				return fmt.Errorf("%w: generated file %q references unknown workload %q", ErrInvalidSpec, file.Name, file.WorkloadID)
			}
		} else if len(plan.Workloads) != 1 {
			return fmt.Errorf("%w: generated file %q must name its workload when a plan has multiple workloads", ErrInvalidSpec, file.Name)
		}
		fileNames[file.Name], fileTargets[file.Target] = struct{}{}, struct{}{}
	}
	storageBindings := make(map[string]struct{}, len(plan.Storage))
	for _, attachment := range plan.Storage {
		if _, exists := ids[attachment.WorkloadID]; !exists || attachment.Binding == "" || attachment.Target == "" || !strings.HasPrefix(attachment.Target, "/") {
			return fmt.Errorf("%w: storage attachment requires a workload, binding and absolute target", ErrInvalidSpec)
		}
		key := attachment.WorkloadID + "\x00" + attachment.Binding
		if _, exists := storageBindings[key]; exists {
			return fmt.Errorf("%w: workload %q repeats storage binding %q", ErrInvalidSpec, attachment.WorkloadID, attachment.Binding)
		}
		storageBindings[key] = struct{}{}
		if err := addPlanMountTarget(mountTargets[attachment.WorkloadID], attachment.Target); err != nil {
			return fmt.Errorf("%w: workload %q storage target %q conflicts with another attachment", ErrInvalidSpec, attachment.WorkloadID, attachment.Target)
		}
	}
	for _, file := range plan.Files {
		workloadID := file.WorkloadID
		if workloadID == "" {
			workloadID = plan.Workloads[0].ID
		}
		if err := addPlanMountTarget(mountTargets[workloadID], file.Target); err != nil {
			return fmt.Errorf("%w: generated file %q conflicts with workload %q attachment", ErrInvalidSpec, file.Name, workloadID)
		}
	}
	return nil
}

func addPlanMountTarget(targets map[string]struct{}, target string) error {
	clean := filepathClean(target)
	if _, exists := targets[clean]; exists {
		return ErrInvalidSpec
	}
	targets[clean] = struct{}{}
	return nil
}

func filepathClean(value string) string { return path.Clean(value) }

// InfrastructureEngine is the executor-agnostic resource-group boundary.
// Concrete engines realize plans using provider primitives only.
type InfrastructureEngine interface {
	Capabilities() Capabilities
	ValidatePlan(context.Context, WorkloadPlan) error
	ResolveStorage(context.Context, StorageRequest) ([]StorageBinding, error)
	ValidateStorageAccess(context.Context, StorageAccessRequest) error
	StartPlan(context.Context, WorkloadPlan) (ResourceGroup, error)
	InspectGroup(context.Context, GroupRef) (GroupObservation, error)
	ListGroups(context.Context, Selector) ([]ResourceGroup, error)
	StopGroup(context.Context, GroupRef) error
	CleanupGroup(context.Context, GroupRef) error
	Close() error
}

// LogForwarder consumes a workload's output stream. Implementations own
// application-specific parsing and redaction; engines only own stream lifetime.
type LogForwarder func(io.Reader, ...any)

func ForwardLogs(forward LogForwarder, reader io.Reader, attributes ...any) {
	if forward == nil {
		_, _ = io.Copy(io.Discard, reader)
		return
	}
	forward(reader, attributes...)
}

type StorageRequest struct {
	Owner      string
	Generation string
	Bindings   []StorageBindingRequest
}

type StorageAccessRequest struct {
	StorageID string
	Subpath   string
}

type StorageBindingRequest struct {
	Name       string
	StorageID  string
	Subpath    string
	Durable    bool
	Create     bool
	PlatformIO bool
}

type StorageBinding struct {
	Name         string
	Identity     string
	Subpath      string
	HostPath     string
	ProviderPath string
	ProviderID   string
}

type GroupRef struct {
	Identity GroupIdentity
	Provider string
}

type ResourceGroup struct {
	Ref        GroupRef
	Components []Resource
	Endpoints  map[string]string
	Metadata   map[string]string
}

type GroupObservation struct {
	State GroupState
	Group ResourceGroup
}

const groupHandlePrefix = "orkest-group-v1:"

// EncodeGroupRef creates a versioned opaque handle suitable for existing
// process-identity wire fields while retaining enough information to recover a
// group after the coordinating executor is restarted.
func EncodeGroupRef(ref GroupRef) (string, error) {
	data, err := json.Marshal(ref)
	if err != nil {
		return "", fmt.Errorf("encode infrastructure group reference: %w", err)
	}
	return groupHandlePrefix + base64.RawURLEncoding.EncodeToString(data), nil
}

// DecodeGroupRef parses the versioned generic handle format.
func DecodeGroupRef(value string) (GroupRef, error) {
	encoded, ok := strings.CutPrefix(value, groupHandlePrefix)
	if !ok || encoded == "" {
		return GroupRef{}, fmt.Errorf("%w: unsupported infrastructure group handle", ErrInvalidSpec)
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return GroupRef{}, fmt.Errorf("%w: invalid infrastructure group handle", ErrInvalidSpec)
	}
	var ref GroupRef
	if err = json.Unmarshal(data, &ref); err != nil || ref.Identity.InstallationID == "" || ref.Identity.Owner == "" || ref.Identity.Generation == "" || ref.Provider == "" {
		return GroupRef{}, fmt.Errorf("%w: incomplete infrastructure group handle", ErrInvalidSpec)
	}
	return ref, nil
}

type GroupState uint8

const (
	GroupAbsent GroupState = iota
	GroupActive
	GroupStopped
	GroupUncertain
)

type RestartPolicy uint8

const (
	RestartNever RestartPolicy = iota
	RestartOnFailure
)

type Requirements struct {
	Mode                     ExecutionMode
	DurableWork              bool
	DurableState             bool
	ReadOnlyRoot             bool
	ReadOnlyMounts           bool
	TemporaryMounts          bool
	ProtectedStorageSharing  bool
	PlatformIngress          bool
	DeniedEgress             bool
	PrivateServiceRouting    bool
	ConfirmedProcessTreeStop bool
	ExplicitHostBinds        bool
	CredentialBindings       bool
	RestartModes             bool
}

type Capabilities struct {
	Modes                    []ExecutionMode
	DurableWork              bool
	DurableState             bool
	PlatformStorageSetup     bool
	ReadOnlyRoot             bool
	ReadOnlyMounts           bool
	TemporaryMounts          bool
	ProtectedStorageSharing  bool
	PlatformIngress          bool
	DeniedEgress             bool
	PrivateServiceRouting    bool
	ConfirmedProcessTreeStop bool
	ExplicitHostBinds        bool
	CredentialBindings       bool
	RestartModes             bool
}

// MissingCapabilities reports each requested guarantee the engine does not
// advertise. Live provider health remains a separate runtime qualification.
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
		{"protected storage sharing", r.ProtectedStorageSharing, c.ProtectedStorageSharing},
		{"platform ingress", r.PlatformIngress, c.PlatformIngress},
		{"denied egress", r.DeniedEgress, c.DeniedEgress},
		{"private service routing", r.PrivateServiceRouting, c.PrivateServiceRouting},
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

type Resource struct {
	Ref   ResourceRef
	State ResourceState
}

var (
	ErrUnsupported      = errors.New("infrastructure requirement is unsupported")
	ErrInvalidSpec      = errors.New("invalid workload plan")
	ErrIdentityMismatch = errors.New("infrastructure resource identity mismatch")
	ErrUncertain        = errors.New("infrastructure operation outcome is uncertain")
)

func Unsupported(requirement string) error {
	return fmt.Errorf("%w: %s", ErrUnsupported, requirement)
}
