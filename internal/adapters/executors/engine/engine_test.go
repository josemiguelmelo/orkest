package engine

import (
	"errors"
	"reflect"
	"testing"
)

func TestCapabilitiesMissingReportsOnlyUnsupportedRequirements(t *testing.T) {
	capabilities := Capabilities{
		Modes:              []ExecutionMode{ExecutionModeContainer},
		DurableWork:        true,
		DurableState:       true,
		ReadOnlyMounts:     true,
		CredentialBindings: true,
	}
	requirements := Requirements{
		Mode:            ExecutionModeNative,
		DurableWork:     true,
		DurableState:    true,
		ReadOnlyMounts:  true,
		PlatformIngress: true,
		DeniedEgress:    true,
	}
	want := []string{"requested execution mode", "platform ingress", "denied egress"}
	if got := capabilities.Missing(requirements); !reflect.DeepEqual(got, want) {
		t.Fatalf("Missing() = %v, want %v", got, want)
	}
}

func TestCapabilitiesUnknownModeDoesNotRequireModeSupport(t *testing.T) {
	if got := (Capabilities{}).Missing(Requirements{}); len(got) != 0 {
		t.Fatalf("Missing() = %v, want no missing capabilities", got)
	}
	if err := ValidateCapabilities(Capabilities{}, Requirements{}); err != nil {
		t.Fatalf("ValidateCapabilities() = %v, want nil", err)
	}
	if err := ValidateCapabilities(Capabilities{}, Requirements{DeniedEgress: true}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ValidateCapabilities() = %v, want ErrUnsupported", err)
	}
}

func TestValidateWorkloadPlanUsesOnlyInfrastructureIdentity(t *testing.T) {
	plan := WorkloadPlan{
		Group:    GroupIdentity{InstallationID: "install", Owner: "opaque-owner", Generation: "gen-1"},
		Metadata: map[string]string{"caller": "future-executor"},
		Workloads: []Workload{
			{ID: "web", Mode: ExecutionModeContainer, Artifact: Artifact{ContainerImage: "web:1"}, Endpoints: []Endpoint{{Name: "web-http", Protocol: "http", ListenPort: 8080}}},
			{ID: "gateway", Mode: ExecutionModeContainer, Artifact: Artifact{ContainerImage: "gateway:1"}},
		},
		Networks: []NetworkPlan{{ID: "private", Members: []string{"web", "gateway"}, DenyEgress: true}},
		Probes:   []Probe{{WorkloadID: "web", Endpoint: "web-http", Path: "/ready", StatusCode: 200}},
	}
	if err := ValidateWorkloadPlan(plan); err != nil {
		t.Fatalf("valid generic plan rejected: %v", err)
	}
	plan.Networks[0].Members[1] = "missing"
	if err := ValidateWorkloadPlan(plan); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("unknown workload reference error = %v", err)
	}
}

func TestGroupHandleRoundTripsGenericRecoveryIdentity(t *testing.T) {
	want := GroupRef{Identity: GroupIdentity{InstallationID: "install", Owner: "opaque-owner", Generation: "generation-4"}, Provider: "provider-resource-id"}
	encoded, err := EncodeGroupRef(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeGroupRef(encoded)
	if err != nil || got != want {
		t.Fatalf("DecodeGroupRef() = %#v, %v; want %#v", got, err, want)
	}
	if _, err = DecodeGroupRef("unsupported-process-identity"); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("DecodeGroupRef(unsupported identity) = %v", err)
	}
}

func TestValidateWorkloadPlanChecksCrossWorkloadEndpointAndMountReferences(t *testing.T) {
	plan := WorkloadPlan{
		Group: GroupIdentity{Owner: "owner", Generation: "generation"},
		Workloads: []Workload{
			{ID: "web", Mode: ExecutionModeContainer, Artifact: Artifact{ContainerImage: "web:1"}, Endpoints: []Endpoint{{Name: "http", Protocol: "http", ListenPort: 8080}}},
			{ID: "router", Mode: ExecutionModeContainer, Artifact: Artifact{ContainerImage: "router:1"}, Arguments: []Argument{{Kind: ArgumentEndpointAddress, Workload: "web", Endpoint: "http"}}},
		},
		Networks: []NetworkPlan{{ID: "private", Members: []string{"web", "router"}, DenyEgress: true}},
	}
	if err := ValidateWorkloadPlan(plan); err != nil {
		t.Fatalf("valid cross-workload endpoint reference rejected: %v", err)
	}
	plan.Files = []GeneratedFile{{Name: "router-config", Target: "/etc/router/config.json", Contents: []byte("{}"), Mode: 0o444}}
	if err := ValidateWorkloadPlan(plan); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("unassigned generated file in a multi-workload plan error = %v", err)
	}
	plan.Files[0].WorkloadID = "router"
	if err := ValidateWorkloadPlan(plan); err != nil {
		t.Fatalf("explicitly assigned generated file rejected: %v", err)
	}
	plan.Files = nil
	plan.Workloads[1].Arguments[0].Endpoint = "missing"
	if err := ValidateWorkloadPlan(plan); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("unknown cross-workload endpoint error = %v", err)
	}
	plan.Workloads[1].Arguments = nil
	plan.Workloads[0].Mounts = []Mount{{SourceKind: MountSourceHost, Source: "/source", Target: "/data"}}
	plan.Storage = []StorageAttachment{{WorkloadID: "web", Binding: "data", StorageID: "storage", Target: "/data", Create: true}}
	if err := ValidateWorkloadPlan(plan); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("duplicate writable mount target error = %v", err)
	}
	plan.Workloads[0].Mounts[0].ReadOnly = true
	plan.Storage[0].Target = "/data/child"
	if err := ValidateWorkloadPlan(plan); err != nil {
		t.Fatalf("explicit writable child under protected read-only ancestry rejected: %v", err)
	}
}
