package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
)

type groupRecord struct {
	Identity  engine.GroupIdentity `json:"identity"`
	ProcessID string               `json:"process_id"`
	Workload  string               `json:"workload"`
	Endpoints map[string]string    `json:"endpoints,omitempty"`
}

var _ engine.InfrastructureEngine = (*Engine)(nil)

func (e *Engine) ValidateStorageAccess(ctx context.Context, _ engine.StorageAccessRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return engine.Unsupported("Local managed workload storage access inspection")
}

func (e *Engine) ValidatePlan(ctx context.Context, plan engine.WorkloadPlan) error {
	if err := engine.ValidateWorkloadPlan(plan); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if plan.Group.InstallationID != "" && plan.Group.InstallationID != e.installation {
		return engine.ErrIdentityMismatch
	}
	if len(plan.Workloads) != 1 || len(plan.Networks) != 0 || len(plan.Files) != 0 || len(plan.Probes) != 0 {
		return engine.Unsupported("Local native workload group shape")
	}
	workload := plan.Workloads[0]
	if workload.Mode != engine.ExecutionModeNative {
		return engine.Unsupported("Local native workloads")
	}
	if !filepath.IsAbs(workload.Artifact.ExecutablePath) || !filepath.IsAbs(workload.WorkingPath) {
		return fmt.Errorf("%w: native executable and working directory must be absolute", engine.ErrInvalidSpec)
	}
	if len(workload.Mounts) != 0 || len(workload.TemporaryMounts) != 0 || len(workload.Credentials) != 0 || workload.Restart != engine.RestartNever {
		return engine.Unsupported("Local mounts, credentials and restart policies")
	}
	if workload.Security != (engine.SecurityRequirements{}) || workload.Limits != (engine.ResourceLimits{}) {
		return engine.Unsupported("Local process isolation and resource limits")
	}
	for _, endpoint := range workload.Endpoints {
		if endpoint.ListenHost != "" && endpoint.ListenHost != "127.0.0.1" && endpoint.ListenHost != "localhost" {
			return engine.Unsupported("Local endpoints must bind loopback")
		}
		if endpoint.Exposure != engine.ExposurePrivate && endpoint.Exposure != engine.ExposurePlatform {
			return engine.Unsupported("Local endpoint ingress policy")
		}
	}
	if _, err := os.Stat(workload.WorkingPath); err != nil {
		return fmt.Errorf("inspect native workload directory: %w", err)
	}
	program, err := os.Stat(workload.Artifact.ExecutablePath)
	if err != nil {
		return fmt.Errorf("inspect native workload executable: %w", err)
	}
	if program.IsDir() || program.Mode()&0o111 == 0 {
		return fmt.Errorf("%w: native executable must be an executable file", engine.ErrInvalidSpec)
	}
	return nil
}

func (e *Engine) ResolveStorage(ctx context.Context, request engine.StorageRequest) ([]engine.StorageBinding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request.Owner == "" || request.Generation == "" {
		return nil, fmt.Errorf("%w: storage owner and generation are required", engine.ErrInvalidSpec)
	}
	bindings := make([]engine.StorageBinding, 0, len(request.Bindings))
	for _, binding := range request.Bindings {
		if binding.Name == "" {
			return nil, fmt.Errorf("%w: storage binding name is required", engine.ErrInvalidSpec)
		}
		storageID := binding.StorageID
		subpath := binding.Subpath
		if storageID == "" {
			if subpath != "" {
				return nil, fmt.Errorf("%w: storage subpath requires a stable storage identity", engine.ErrInvalidSpec)
			}
			storageID = taskstorage.Identity(request.Owner, request.Generation, binding.Name)
		}
		workPath, pathErr := taskstorage.BindingPath(e.dataRoot, storageID, subpath)
		if pathErr != nil {
			return nil, fmt.Errorf("%w: invalid Local storage identity or subpath", engine.ErrInvalidSpec)
		}
		if binding.Create {
			if err := os.MkdirAll(workPath, 0o700); err != nil {
				return nil, fmt.Errorf("create local storage binding: %w", err)
			}
		} else if _, err := os.Stat(workPath); err != nil {
			return nil, fmt.Errorf("inspect local storage binding: %w", err)
		}
		bindings = append(bindings, engine.StorageBinding{Name: binding.Name, Identity: storageID, Subpath: subpath, HostPath: workPath})
	}
	return bindings, nil
}

func (e *Engine) StartPlan(ctx context.Context, plan engine.WorkloadPlan) (engine.ResourceGroup, error) {
	if err := e.ValidatePlan(ctx, plan); err != nil {
		return engine.ResourceGroup{}, err
	}
	plan.Group.InstallationID = e.installation
	return engine.RunRecordedStart(ctx, e.manifests, plan, "local", e.startPlanResources)
}

func (e *Engine) startPlanResources(ctx context.Context, plan engine.WorkloadPlan) (engine.ResourceGroup, error) {
	groupID := groupID(plan.Group)
	if current, err := e.readGroup(groupID); err == nil {
		observation, inspectErr := e.inspectGroupRecord(ctx, groupID, current)
		if inspectErr != nil {
			return engine.ResourceGroup{}, inspectErr
		}
		if observation.State != engine.GroupAbsent {
			return observation.Group, engine.ErrUncertain
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return engine.ResourceGroup{}, err
	}
	storageRequests := make([]engine.StorageBindingRequest, 0, len(plan.Storage))
	for _, attachment := range plan.Storage {
		storageRequests = append(storageRequests, engine.StorageBindingRequest{Name: attachment.Binding, StorageID: attachment.StorageID, Subpath: attachment.Subpath, Durable: attachment.Durable, Create: attachment.Create, PlatformIO: attachment.PlatformIO})
	}
	storageBindings, err := e.ResolveStorage(ctx, engine.StorageRequest{Owner: plan.Group.Owner, Generation: plan.Group.Generation, Bindings: storageRequests})
	if err != nil {
		return engine.ResourceGroup{}, err
	}
	storageByName := make(map[string]engine.StorageBinding, len(storageBindings))
	for _, binding := range storageBindings {
		storageByName[binding.Name] = binding
	}
	for _, attachment := range plan.Storage {
		binding, exists := storageByName[attachment.Binding]
		if !exists || attachment.Target != binding.HostPath || attachment.ReadOnly {
			return engine.ResourceGroup{}, engine.Unsupported("Local storage attachments must use the resolved host path with writable access")
		}
	}
	workload := plan.Workloads[0]
	endpointAddresses := make(map[string]string, len(workload.Endpoints))
	endpointURLs := make(map[string]string, len(workload.Endpoints))
	for _, endpoint := range workload.Endpoints {
		host := endpoint.ListenHost
		if host == "" || host == "localhost" {
			host = "127.0.0.1"
		}
		port := endpoint.ListenPort
		if port == 0 {
			listener, listenErr := net.Listen("tcp", net.JoinHostPort(host, "0"))
			if listenErr != nil {
				return engine.ResourceGroup{}, fmt.Errorf("reserve local endpoint: %w", listenErr)
			}
			port = listener.Addr().(*net.TCPAddr).Port
			if listenErr = listener.Close(); listenErr != nil {
				return engine.ResourceGroup{}, fmt.Errorf("release local endpoint: %w", listenErr)
			}
		}
		address := net.JoinHostPort(host, fmt.Sprintf("%d", port))
		endpointAddresses[endpoint.Name] = address
		if endpoint.Protocol == "http" || endpoint.Protocol == "https" {
			endpointURLs[endpoint.Name] = endpoint.Protocol + "://" + address
		} else {
			endpointURLs[endpoint.Name] = address
		}
	}
	args := make([]string, 0, len(workload.Arguments))
	for _, argument := range workload.Arguments {
		if argument.Kind == engine.ArgumentLiteral {
			args = append(args, argument.Value)
			continue
		}
		args = append(args, endpointAddresses[argument.Endpoint])
	}
	cmd := exec.Command(workload.Artifact.ExecutablePath, args...)
	cmd.Dir = workload.WorkingPath
	cmd.Stdin, cmd.Stdout = nil, io.Discard
	cmd.Env = environmentEntries(workload.Environment)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return engine.ResourceGroup{}, fmt.Errorf("capture workload logs: %w", err)
	}
	if err = cmd.Start(); err != nil {
		return engine.ResourceGroup{}, fmt.Errorf("start native workload: %w", err)
	}
	identity, err := processIdentity(ctx, cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return engine.ResourceGroup{}, fmt.Errorf("identify native workload: %w", err)
	}
	processID := fmt.Sprintf("%d:%s", cmd.Process.Pid, identity)
	record := groupRecord{Identity: plan.Group, ProcessID: processID, Workload: workload.ID, Endpoints: endpointURLs}
	if err = e.writeGroup(groupID, record); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return engine.ResourceGroup{}, err
	}
	e.mu.Lock()
	e.processes[processID] = cmd.Process
	e.mu.Unlock()
	logsDone := make(chan struct{})
	go func() {
		engine.ForwardLogs(e.logForwarder, stderr, "owner", plan.Group.Owner, "generation", plan.Group.Generation)
		close(logsDone)
	}()
	go func() {
		<-logsDone
		_ = cmd.Wait()
		e.mu.Lock()
		delete(e.processes, processID)
		e.mu.Unlock()
	}()
	return groupResource(record, groupID, e.installation, engine.ResourceRunning), nil
}

func (e *Engine) InspectGroup(ctx context.Context, ref engine.GroupRef) (engine.GroupObservation, error) {
	if ref.Identity.InstallationID != e.installation {
		return engine.GroupObservation{}, engine.ErrIdentityMismatch
	}
	id := groupID(ref.Identity)
	record, err := e.readGroup(id)
	if errors.Is(err, os.ErrNotExist) {
		return engine.GroupObservation{State: engine.GroupAbsent}, nil
	}
	if err != nil {
		return engine.GroupObservation{}, err
	}
	if ref.Provider != "" && ref.Provider != record.ProcessID {
		return engine.GroupObservation{}, engine.ErrIdentityMismatch
	}
	return e.inspectGroupRecord(ctx, id, record)
}

func (e *Engine) inspectGroupRecord(ctx context.Context, id string, record groupRecord) (engine.GroupObservation, error) {
	if err := ctx.Err(); err != nil {
		return engine.GroupObservation{}, err
	}
	pid, fingerprint, err := parseIdentity(record.ProcessID)
	if err != nil {
		return engine.GroupObservation{}, err
	}
	actual, err := processIdentity(ctx, pid)
	state := engine.GroupActive
	resourceState := engine.ResourceRunning
	if errors.Is(err, errProcessMissing) {
		state, resourceState = engine.GroupStopped, engine.ResourceStopped
	} else if err != nil {
		return engine.GroupObservation{}, err
	} else if actual != fingerprint {
		terminated, stateErr := processTerminated(ctx, pid)
		if stateErr != nil {
			return engine.GroupObservation{}, stateErr
		}
		if !terminated {
			return engine.GroupObservation{}, engine.ErrIdentityMismatch
		}
		state, resourceState = engine.GroupStopped, engine.ResourceStopped
	}
	return engine.GroupObservation{State: state, Group: groupResource(record, id, e.installation, resourceState)}, nil
}

func (e *Engine) ListGroups(ctx context.Context, selector engine.Selector) ([]engine.ResourceGroup, error) {
	if selector.InstallationID != "" && selector.InstallationID != e.installation {
		return nil, engine.ErrIdentityMismatch
	}
	paths, err := filepath.Glob(filepath.Join(e.dataRoot, "groups", "*.json"))
	if err != nil {
		return nil, err
	}
	groups := make([]engine.ResourceGroup, 0, len(paths))
	for _, path := range paths {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		data, readErr := os.ReadFile(path)
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return nil, readErr
		}
		var record groupRecord
		if readErr = json.Unmarshal(data, &record); readErr != nil {
			return nil, fmt.Errorf("decode native workload group: %w", readErr)
		}
		if record.Identity.InstallationID != e.installation || selector.GroupOwner != "" && selector.GroupOwner != record.Identity.Owner {
			continue
		}
		observation, inspectErr := e.inspectGroupRecord(ctx, groupID(record.Identity), record)
		if inspectErr != nil {
			return nil, inspectErr
		}
		groups = append(groups, observation.Group)
	}
	return groups, nil
}

func (e *Engine) StopGroup(ctx context.Context, ref engine.GroupRef) error {
	return engine.RunRecordedLifecycle(ctx, e.manifests, ref.Identity, "stopped", "local", func() error {
		observation, err := e.InspectGroup(ctx, ref)
		if err != nil || observation.State == engine.GroupAbsent || observation.State == engine.GroupStopped {
			return err
		}
		component := observation.Group.Components[0]
		pid, token, parseErr := parseIdentity(component.Ref.ConcreteID)
		if parseErr != nil {
			return engine.ErrIdentityMismatch
		}
		identity, inspectErr := processIdentity(ctx, pid)
		if errors.Is(inspectErr, errProcessMissing) {
			return nil
		}
		if inspectErr != nil || identity != token {
			return engine.ErrIdentityMismatch
		}
		if err = signalProcess(pid); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		deadline := time.NewTicker(25 * time.Millisecond)
		defer deadline.Stop()
		for {
			terminated, stateErr := processTerminated(ctx, pid)
			if stateErr != nil {
				return stateErr
			}
			if terminated {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-deadline.C:
			}
		}
	})
}

func (e *Engine) CleanupGroup(ctx context.Context, ref engine.GroupRef) error {
	return engine.RunRecordedLifecycle(ctx, e.manifests, ref.Identity, "cleaned", "local", func() error {
		observation, err := e.InspectGroup(ctx, ref)
		if err != nil || observation.State == engine.GroupAbsent {
			return err
		}
		if observation.State != engine.GroupStopped {
			return errors.New("refusing to remove active native workload group")
		}
		id := groupID(ref.Identity)
		if err = os.Remove(e.groupPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	})
}

func groupResource(record groupRecord, id, installation string, state engine.ResourceState) engine.ResourceGroup {
	key := engine.ResourceKey{InstallationID: installation, ResourceID: id, ComponentID: record.Workload}
	resource := engine.Resource{Ref: engine.ResourceRef{Key: key, ConcreteID: record.ProcessID}, State: state}
	ref := engine.GroupRef{Identity: record.Identity, Provider: record.ProcessID}
	return engine.ResourceGroup{Ref: ref, Components: []engine.Resource{resource}, Endpoints: record.Endpoints, Metadata: map[string]string{"workload": record.Workload}}
}

func groupID(identity engine.GroupIdentity) string {
	hash := sha256.Sum256([]byte(identity.Owner + "\x00" + identity.Generation))
	return hex.EncodeToString(hash[:])
}

func (e *Engine) groupPath(id string) string {
	return filepath.Join(e.dataRoot, "groups", id+".json")
}

func (e *Engine) readGroup(id string) (groupRecord, error) {
	data, err := os.ReadFile(e.groupPath(id))
	if err != nil {
		return groupRecord{}, err
	}
	var record groupRecord
	if err = json.Unmarshal(data, &record); err != nil {
		return groupRecord{}, err
	}
	return record, nil
}

func (e *Engine) writeGroup(id string, record groupRecord) error {
	path := e.groupPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err = os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	if err = os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func environmentEntries(values map[string]string) []string {
	entries := make([]string, 0, len(values))
	for name, value := range values {
		if strings.TrimSpace(name) != "" && !strings.Contains(name, "=") {
			entries = append(entries, name+"="+value)
		}
	}
	return entries
}
