package docker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/josemiguelmelo/orkest/internal/platform/repositorypath"
	"github.com/josemiguelmelo/orkest/internal/platform/taskstorage"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	client "github.com/moby/moby/client"
)

const (
	checkoutMountLayoutLabel = "orkest.checkout_mount_layout"
	checkoutMountLayout      = "protected-v1"
)

// Pin repository ancestry behind a read-only bind, then expose only checkout
// contents as writable mounts at their original paths. A worker cannot rename
// those mountpoints or their parents while Docker resolves an editor bind.
func protectedCheckoutMounts(work string, repositories []orchestrationmodel.WorkspaceRepository) ([]mount.Mount, error) {
	parent := filepath.Join(work, "repositories")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, fmt.Errorf("create checkout mount parent: %w", err)
	}
	if _, err := existingDirectory(work, parent); err != nil {
		return nil, fmt.Errorf("validate checkout mount parent: %w", err)
	}
	mounts := []mount.Mount{{Type: mount.TypeBind, Source: parent, Target: workMountPath + "/repositories", ReadOnly: true, BindOptions: &mount.BindOptions{ReadOnlyNonRecursive: true}}}
	for _, repository := range repositories {
		relative, err := repositorypath.Relative(repository.ProviderName, repository.ID)
		if err != nil {
			return nil, err
		}
		source, err := existingDirectory(work, filepath.Join(work, relative))
		if err != nil {
			return nil, fmt.Errorf("validate checkout mount: %w", err)
		}
		mounts = append(mounts, mount.Mount{Type: mount.TypeBind, Source: source, Target: workMountPath + "/" + filepath.ToSlash(relative)})
	}
	return mounts, nil
}

// The task delivery guard excludes new worker creation while editor admission
// runs. Existing legacy workers keep running, but cannot safely share an editor.
// Check stopped containers too: their restart policy may bring them back alive.
func (b *EditorBackend) requireProtectedWorkers(ctx context.Context, spec taskeditor.Spec) error {
	filters := client.Filters{}.Add("label", "orkest.managed=agentctl")
	for key, value := range map[string]string{"organization": spec.Scope.OrganizationID, "workspace": spec.Scope.WorkspaceID, "task": spec.Scope.TaskID} {
		hash, err := hashScope(value)
		if err != nil {
			return err
		}
		filters = filters.Add("label", "orkest."+key+"_id_hash="+hash)
	}
	listed, err := b.engine.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: filters})
	if err != nil {
		return err
	}
	work, _ := taskstorage.Directories(b.executor.config.DataRoot, spec.Scope.OrganizationID, spec.Scope.WorkspaceID, spec.Scope.TaskID)
	work, err = filepath.EvalSymlinks(work)
	if err != nil {
		return err
	}
	for _, summary := range listed.Items {
		inspected, err := b.engine.ContainerInspect(ctx, summary.ID, client.ContainerInspectOptions{})
		if errdefs.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		if err := protectedWorker(inspected.Container, work, spec.Discovery.Roots); err != nil {
			return fmt.Errorf("worker checkout mounts prevent editor admission: %w", errors.Join(taskeditor.ErrUnavailable, err))
		}
	}
	return nil
}

func protectedWorker(worker container.InspectResponse, work string, roots []taskeditor.Root) error {
	if worker.Config == nil || worker.Config.Labels[checkoutMountLayoutLabel] != checkoutMountLayout || worker.HostConfig == nil || worker.HostConfig.Privileged {
		return errors.New("worker uses an unprotected checkout layout")
	}
	for _, capability := range worker.HostConfig.CapAdd {
		capability = strings.TrimPrefix(strings.ToUpper(capability), "CAP_")
		if capability == "SYS_ADMIN" || capability == "ALL" {
			return errors.New("worker can change checkout mounts")
		}
	}
	protected := false
	repositories := filepath.Join(work, "repositories")
	for _, m := range worker.Mounts {
		source, err := filepath.EvalSymlinks(m.Source)
		if err != nil {
			return fmt.Errorf("resolve worker mount: %w", err)
		}
		if m.Destination == workMountPath+"/repositories" && source == filepath.Join(work, "repositories") && !m.RW {
			protected = true
		}
		if !m.RW || (m.Destination == workMountPath && source == work) {
			continue
		}
		if within(work, source) {
			if !within(repositories, source) || source == repositories {
				return errors.New("worker has a writable alias to task workspace")
			}
			relative, relErr := filepath.Rel(work, source)
			if relErr != nil || m.Destination != workMountPath+"/"+filepath.ToSlash(relative) {
				return errors.New("worker has a writable alias to checkout ancestry")
			}
			parts := strings.Split(filepath.ToSlash(relative), "/")
			if len(parts) != 3 || parts[0] != "repositories" {
				return errors.New("worker has a writable alias to checkout ancestry")
			}
		}
		if within(source, work) {
			return errors.New("worker has a writable alias to task workspace ancestry")
		}
		for _, root := range roots {
			if source != root.Source && within(source, root.Source) {
				return errors.New("worker has a writable alias to checkout ancestry")
			}
		}
	}
	if !protected {
		return errors.New("worker checkout parents are writable")
	}
	return nil
}
