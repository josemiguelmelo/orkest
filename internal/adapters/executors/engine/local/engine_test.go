package local

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
)

func TestInfrastructureEngineLifecycleForArbitraryNativeWorkload(t *testing.T) {
	root := t.TempDir()
	engineInstance, err := New(Config{DataRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := engineInstance.Close(); closeErr != nil {
			t.Errorf("close local engine: %v", closeErr)
		}
	}()
	shell, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep executable is not available")
	}
	storage, err := engineInstance.ResolveStorage(context.Background(), engine.StorageRequest{
		Owner: "opaque-owner", Generation: "generation-1",
		Bindings: []engine.StorageBindingRequest{{Name: "work", Create: true, Durable: true}},
	})
	if err != nil || len(storage) != 1 {
		t.Fatalf("resolve storage = %#v, %v", storage, err)
	}
	plan := engine.WorkloadPlan{
		Group:    engine.GroupIdentity{InstallationID: engineInstance.installation, Owner: "opaque-owner", Generation: "generation-1"},
		Metadata: map[string]string{"kind": "custom-tool"},
		Storage:  []engine.StorageAttachment{{WorkloadID: "arbitrary-component", Binding: "work", Target: storage[0].HostPath, Durable: true}},
		Workloads: []engine.Workload{{
			ID: "arbitrary-component", Mode: engine.ExecutionModeNative,
			Artifact:    engine.Artifact{ExecutablePath: shell},
			Endpoints:   []engine.Endpoint{{Name: "control", Protocol: "http", ListenHost: "127.0.0.1", Exposure: engine.ExposurePlatform}},
			Arguments:   []engine.Argument{{Kind: engine.ArgumentLiteral, Value: "30"}},
			WorkingPath: storage[0].HostPath,
		}},
	}
	ctx := context.Background()
	group, err := engineInstance.StartPlan(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	if err = engineInstance.Close(); err != nil {
		t.Fatal(err)
	}
	engineInstance, err = New(Config{DataRoot: root})
	if err != nil {
		t.Fatalf("recreate local engine: %v", err)
	}
	manifest, err := engineInstance.manifests.Read(ctx, group.Ref.Identity)
	if err != nil || manifest.State != "active" || len(manifest.ProviderIDs) != 1 {
		t.Fatalf("recovered operation manifest = %+v, %v", manifest, err)
	}
	if group.Ref.Identity != plan.Group || group.Components[0].State != engine.ResourceRunning {
		t.Fatalf("unexpected group after start: %#v", group)
	}
	if !strings.HasPrefix(group.Endpoints["control"], "http://127.0.0.1:") {
		t.Fatalf("endpoint map = %#v", group.Endpoints)
	}
	listed, err := engineInstance.ListGroups(ctx, engine.Selector{InstallationID: engineInstance.installation, GroupOwner: "opaque-owner"})
	if err != nil || len(listed) != 1 {
		t.Fatalf("list groups = %#v, %v", listed, err)
	}
	observed, err := engineInstance.InspectGroup(ctx, group.Ref)
	if err != nil || observed.State != engine.GroupActive {
		t.Fatalf("inspect group = %#v, %v", observed, err)
	}
	if observed.Group.Endpoints["control"] != group.Endpoints["control"] {
		t.Fatalf("inspected endpoint map = %#v", observed.Group.Endpoints)
	}
	if err = engineInstance.StopGroup(ctx, group.Ref); err != nil {
		t.Fatal(err)
	}
	manifest, err = engineInstance.manifests.Read(ctx, group.Ref.Identity)
	if err != nil || manifest.State != "stopped" {
		t.Fatalf("stopped operation manifest = %+v, %v", manifest, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		observed, err = engineInstance.InspectGroup(ctx, group.Ref)
		if err != nil {
			t.Fatal(err)
		}
		if observed.State == engine.GroupStopped {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if observed.State != engine.GroupStopped {
		t.Fatal(fmt.Errorf("group did not stop: %#v", observed))
	}
	if err = engineInstance.CleanupGroup(ctx, group.Ref); err != nil {
		t.Fatal(err)
	}
	manifest, err = engineInstance.manifests.Read(ctx, group.Ref.Identity)
	if err != nil || manifest.State != "cleaned" {
		t.Fatalf("cleaned operation manifest = %+v, %v", manifest, err)
	}
	observed, err = engineInstance.InspectGroup(ctx, group.Ref)
	if err != nil || observed.State != engine.GroupAbsent {
		t.Fatalf("inspect cleaned group = %#v, %v", observed, err)
	}
}
