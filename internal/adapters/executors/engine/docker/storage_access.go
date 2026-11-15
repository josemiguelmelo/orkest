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

func (e *Engine) ValidateStorageAccess(ctx context.Context, request engine.StorageAccessRequest) error {
	if e.networkClient == nil {
		return engine.Unsupported("Docker managed workload storage inspection")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := taskstorage.BindingPath(e.config.DataRoot, request.StorageID, request.Subpath)
	if err != nil {
		return fmt.Errorf("%w: invalid task storage identity", engine.ErrInvalidSpec)
	}
	work, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve protected Docker storage root: %w", err)
	}
	filters := []client.Filters{client.Filters{}.Add("label", groupLabel)}
	seen := make(map[string]struct{})
	for _, filter := range filters {
		listed, listErr := e.networkClient.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: filter})
		if listErr != nil {
			return fmt.Errorf("list managed Docker workloads for storage admission: %w", listErr)
		}
		for _, item := range listed.Items {
			if _, exists := seen[item.ID]; exists {
				continue
			}
			seen[item.ID] = struct{}{}
			current, inspectErr := e.networkClient.ContainerInspect(ctx, item.ID, client.ContainerInspectOptions{})
			if errdefs.IsNotFound(inspectErr) {
				continue
			}
			if inspectErr != nil {
				return fmt.Errorf("inspect managed Docker workload during storage admission: %w", inspectErr)
			}
			if err = validateManagedStorageWorkload(current.Container, work); err != nil {
				return fmt.Errorf("%w: managed Docker workload storage access cannot be proven safe: %v", engine.ErrUnsupported, err)
			}
		}
	}
	return nil
}

func validateManagedStorageWorkload(workload container.InspectResponse, work string) error {
	if workload.Config == nil || workload.HostConfig == nil {
		return errors.New("managed workload ownership or security settings are unavailable")
	}
	intersects, writable := false, false
	for _, item := range workload.Mounts {
		source, err := filepath.EvalSymlinks(item.Source)
		if err != nil {
			if item.RW {
				return fmt.Errorf("resolve writable workload mount source: %w", err)
			}
			continue
		}
		if pathWithin(work, source) || pathWithin(source, work) {
			intersects = true
			writable = writable || item.RW
		}
	}
	if !intersects {
		return nil
	}
	if workload.HostConfig.Privileged {
		return errors.New("managed workload can alter storage mount ownership")
	}
	for _, capability := range workload.HostConfig.CapAdd {
		if capability == "ALL" || capability == "SYS_ADMIN" || capability == "CAP_SYS_ADMIN" {
			return errors.New("managed workload can alter storage mount ownership")
		}
	}
	if !writable {
		return nil
	}
	return validateProtectedWorkloadMounts(workload, work)
}

func validateProtectedWorkloadMounts(workload container.InspectResponse, work string) error {
	work, err := filepath.EvalSymlinks(work)
	if err != nil {
		return err
	}
	rootTarget := ""
	protected := false
	for _, item := range workload.Mounts {
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
	repositories := filepath.Join(work, "repositories")
	for _, item := range workload.Mounts {
		source, resolveErr := filepath.EvalSymlinks(item.Source)
		if resolveErr != nil {
			return fmt.Errorf("resolve workload mount source: %w", resolveErr)
		}
		if source == repositories && !item.RW && filepath.Clean(item.Destination) == filepath.Join(rootTarget, "repositories") {
			protected = true
		}
		if !item.RW || source == work {
			continue
		}
		if pathWithin(work, source) {
			if !pathWithin(repositories, source) || source == repositories {
				return errors.New("workload has a writable task storage alias")
			}
			relative, relErr := filepath.Rel(work, source)
			if relErr != nil || filepath.Clean(item.Destination) != filepath.Join(rootTarget, relative) {
				return errors.New("workload has a writable checkout ancestry alias")
			}
			parts := strings.Split(filepath.ToSlash(relative), "/")
			if len(parts) != 3 || parts[0] != "repositories" {
				return errors.New("workload has an invalid writable storage child")
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
