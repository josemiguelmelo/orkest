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
	listed, listErr := e.networkClient.ContainerList(ctx, client.ContainerListOptions{All: true})
	if listErr != nil {
		return fmt.Errorf("list Docker workloads for storage admission: %w", listErr)
	}
	for _, item := range listed.Items {
		current, inspectErr := e.networkClient.ContainerInspect(ctx, item.ID, client.ContainerInspectOptions{})
		if errdefs.IsNotFound(inspectErr) {
			continue
		}
		if inspectErr != nil {
			return fmt.Errorf("inspect Docker workload during storage admission: %w", inspectErr)
		}
		if err = validateManagedStorageWorkload(current.Container, work); err != nil {
			return fmt.Errorf("%w: Docker workload storage access cannot be proven safe: %v", engine.ErrUnsupported, err)
		}
	}
	return nil
}

func validateManagedStorageWorkload(workload container.InspectResponse, work string) error {
	intersects := false
	for _, item := range workload.Mounts {
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
