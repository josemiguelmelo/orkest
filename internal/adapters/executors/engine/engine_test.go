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
		Mode:               ExecutionModeNative,
		DurableWork:        true,
		DurableState:       true,
		ReadOnlyMounts:     true,
		GatewayOnlyIngress: true,
		DeniedEgress:       true,
	}
	want := []string{"requested execution mode", "gateway-only ingress", "denied egress"}
	if got := capabilities.Missing(requirements); !reflect.DeepEqual(got, want) {
		t.Fatalf("Missing() = %v, want %v", got, want)
	}
}

func TestValidateSpecAcceptsTypedEndpointAndImageDelivery(t *testing.T) {
	spec := LaunchSpec{
		Key:   ResourceKey{Class: ResourceClassAgent, ExecutionID: "run"},
		Class: ResourceClassAgent, Mode: ExecutionModeContainer,
		Artifact:       Artifact{ContainerImage: "agentctl:1", ContainerCommand: []string{"-serve"}},
		Arguments:      []Argument{{Kind: ArgumentLiteral, Value: "-listen"}, {Kind: ArgumentEndpointAddress, Endpoint: "control"}},
		Endpoints:      []Endpoint{{Name: "control", Protocol: "http", ListenHost: "0.0.0.0", ListenPort: 8081, ReadinessPath: "/health", Ingress: IngressWorkerControl}},
		GeneratedFiles: []GeneratedFile{{Name: "manifest", Target: "/task.code-workspace", Mode: 0o444}},
		Mounts:         []Mount{{SourceKind: MountSourceWorkspace, Source: "task-work", Target: "/task", ReadOnly: true}},
	}
	if err := ValidateSpec(spec); err != nil {
		t.Fatal(err)
	}
}

func TestValidateSpecRejectsMissingEndpointAndMixedArtifacts(t *testing.T) {
	base := LaunchSpec{
		Key:   ResourceKey{Class: ResourceClassAgent, ExecutionID: "run"},
		Class: ResourceClassAgent, Mode: ExecutionModeContainer,
		Artifact: Artifact{ContainerImage: "agentctl:1"},
	}
	base.Arguments = []Argument{{Kind: ArgumentEndpointAddress, Endpoint: "missing"}}
	if err := ValidateSpec(base); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("ValidateSpec() error = %v, want ErrInvalidSpec", err)
	}
	base.Arguments = nil
	base.Artifact.ExecutablePath = "/usr/bin/agentctl"
	if err := ValidateSpec(base); !errors.Is(err, ErrInvalidSpec) {
		t.Fatalf("ValidateSpec() error = %v, want ErrInvalidSpec for mixed delivery", err)
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
