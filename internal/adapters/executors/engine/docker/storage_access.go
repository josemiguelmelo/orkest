package docker

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/josemiguelmelo/orkest/internal/adapters/executors/engine"
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
	"github.com/moby/moby/api/types/container"
	client "github.com/moby/moby/client"
)

type storageAccessError struct {
	code     string
	resource string
	cause    error
}

func (e storageAccessError) Error() string {
	return "Docker workload storage access cannot be proven safe"
}

func (e storageAccessError) Unwrap() error { return e.cause }

func (e storageAccessError) FailureCode() string {
	return "storage_access_" + e.code
}

func (e storageAccessError) FailureResource() string { return e.resource }

func accessFailure(code, resource string, cause error) error {
	return storageAccessError{code: code, resource: resource, cause: cause}
}

func (e *Engine) ValidateStorageAccess(ctx context.Context, request engine.StorageAccessRequest) error {
	if e.networkClient == nil {
		return accessFailure("docker_inspection_unavailable", "", engine.Unsupported("Docker managed workload storage inspection"))
	}
	if err := ctx.Err(); err != nil {
		return accessFailure("context_unavailable", "", err)
	}
	root, err := taskstorage.BindingPath(e.config.DataRoot, request.StorageID, request.Subpath)
	if err != nil {
		return accessFailure("invalid_storage_identity", "", fmt.Errorf("%w: invalid task storage identity", engine.ErrInvalidSpec))
	}
	work, err := filepath.EvalSymlinks(root)
	if err != nil {
		return accessFailure("resolve_storage_root", "", fmt.Errorf("resolve protected Docker storage root: %w", err))
	}
	listed, listErr := e.networkClient.ContainerList(ctx, client.ContainerListOptions{All: true})
	if listErr != nil {
		return accessFailure("list_workloads", "", fmt.Errorf("list Docker workloads for storage admission: %w", listErr))
	}
	for _, item := range listed.Items {
		current, inspectErr := e.networkClient.ContainerInspect(ctx, item.ID, client.ContainerInspectOptions{})
		if errdefs.IsNotFound(inspectErr) {
			continue
		}
		if inspectErr != nil {
			return accessFailure("inspect_workload", item.ID, fmt.Errorf("inspect Docker workload during storage admission: %w", inspectErr))
		}
		if !workloadCanAccessStorage(current.Container) {
			continue
		}
		if err = validateManagedStorageWorkload(current.Container, work); err != nil {
			return accessFailure(storageAccessReason(err), strings.TrimPrefix(current.Container.Name, "/"), engine.ErrUnsupported)
		}
	}
	return nil
}

func storageAccessReason(err error) string {
	switch {
	case strings.Contains(err.Error(), "resolve workload mount source"):
		return "resolve_bind_mount"
	case strings.Contains(err.Error(), "ownership or security settings are unavailable"):
		return "unmanaged_workload_mount"
	case strings.Contains(err.Error(), "can alter storage mount ownership"):
		return "unsafe_workload_privileges"
	case strings.Contains(err.Error(), "no read-only storage root"):
		return "missing_readonly_storage_root"
	case strings.Contains(err.Error(), "writable checkout ancestry alias"):
		return "writable_checkout_alias"
	case strings.Contains(err.Error(), "writable protected root or ancestor"):
		return "writable_storage_ancestor"
	case strings.Contains(err.Error(), "writable task storage ancestry alias"):
		return "writable_storage_alias"
	case strings.Contains(err.Error(), "repository parent is writable"):
		return "writable_repository_parent"
	default:
		return "unverified_workload_mount"
	}
}

func workloadCanAccessStorage(workload container.InspectResponse) bool {
	state := workload.State
	if state == nil || state.Running || state.Restarting {
		return true
	}
	if state.Status != "exited" && state.Status != "dead" {
		return true
	}
	if workload.HostConfig == nil {
		return true
	}
	switch workload.HostConfig.RestartPolicy.Name {
	case "always":
		return true
	case "unless-stopped", "on-failure":
		return state.ExitCode != 0
	default:
		// A successful stop/completion cannot restart under these policies.
		return false
	}
}

func validateManagedStorageWorkload(workload container.InspectResponse, work string) error {
	intersects := false
	for _, item := range workload.Mounts {
		if item.Type != "" && item.Type != "bind" {
			continue
		}
		source, err := filepath.EvalSymlinks(item.Source)
		if err != nil {
			return fmt.Errorf("resolve workload mount source: %w", err)
		}
		if pathWithin(work, source) || pathWithin(source, work) {
			intersects = true
		}
	}
	if !intersects {
		return nil
	}
	if workload.Config == nil || workload.HostConfig == nil || !managedStorageLabels(workload.Config.Labels) {
		return errors.New("workload ownership or security settings are unavailable")
	}
	if workload.HostConfig.Privileged {
		return errors.New("managed workload can alter storage mount ownership")
	}
	for _, capability := range workload.HostConfig.CapAdd {
		if capability == "ALL" || capability == "SYS_ADMIN" || capability == "CAP_SYS_ADMIN" {
			return errors.New("managed workload can alter storage mount ownership")
		}
	}
	return validateProtectedWorkloadMounts(workload, work)
}

func managedStorageLabels(labels map[string]string) bool {
	return labels[groupLabel] != "" && labels[groupOwnerLabel] != "" && labels[groupGenerationLabel] != "" && labels[groupInstallLabel] != "" && labels["orkest.infrastructure.component"] != ""
}

func validateProtectedWorkloadMounts(workload container.InspectResponse, work string) error {
	work, err := filepath.EvalSymlinks(work)
	if err != nil {
		return err
	}
	rootTarget := ""
	protected := false
	for _, item := range workload.Mounts {
		if item.Type != "" && item.Type != "bind" {
			continue
		}
		source, resolveErr := filepath.EvalSymlinks(item.Source)
		if resolveErr != nil {
			return fmt.Errorf("resolve workload mount source: %w", resolveErr)
		}
		if source == work && !item.RW {
			rootTarget = filepath.Clean(item.Destination)
		}
	}
	if rootTarget == "" {
		return errors.New("managed workload has no read-only storage root")
	}
	for _, item := range workload.Mounts {
		if item.Type != "" && item.Type != "bind" {
			continue
		}
		source, resolveErr := filepath.EvalSymlinks(item.Source)
		if resolveErr != nil {
			return fmt.Errorf("resolve workload mount source: %w", resolveErr)
		}
		if source == work && !item.RW && filepath.Clean(item.Destination) == rootTarget {
			protected = true
		}
		if !item.RW || source == work {
			continue
		}
		if pathWithin(work, source) {
			relative, relErr := filepath.Rel(work, source)
			if relErr != nil || filepath.Clean(item.Destination) != filepath.Join(rootTarget, relative) {
				return errors.New("workload has a writable checkout ancestry alias")
			}
			if relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return errors.New("workload has a writable protected root or ancestor")
			}
		}
		if pathWithin(source, work) {
			return errors.New("workload has a writable task storage ancestry alias")
		}
	}
	if !protected {
		return errors.New("managed workload repository parent is writable")
	}
	return nil
}
