package docker

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/josemiguelmelo/orkest/internal/platform/sessionpath"
	orchestrationmodel "github.com/josemiguelmelo/orkest/internal/server/orchestration/model"
	"github.com/moby/moby/api/types/mount"
)

func bindMounts(config orchestrationmodel.RunConfig, stateDir string) ([]mount.Mount, error) {
	mounts := make([]mount.Mount, 0, len(config.HostMounts))
	for _, m := range config.HostMounts {
		target := m.Target
		if strings.HasPrefix(target, "~/") {
			relative := strings.TrimPrefix(target, "~/")
			target = path.Join(sessionpath.Home(stateMountPath, config.OrganizationID, config.WorkspaceID, config.TaskID, config.TaskSessionID), relative)
			home := sessionpath.Home(stateDir, config.OrganizationID, config.WorkspaceID, config.TaskID, config.TaskSessionID)
			localTarget := filepath.Join(home, relative)
			parent := filepath.Dir(localTarget)
			if err := prepareMountParent(stateDir, parent); err != nil {
				return nil, err
			}
			info, err := os.Lstat(localTarget)
			if err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("inspect home mount target: %w", err)
			}
			if err == nil && info.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("home mount target must not be a symlink")
			}
		}
		mounts = append(mounts, mount.Mount{Type: mount.TypeBind, Source: m.Source, Target: target, ReadOnly: m.ReadOnly})
	}
	return mounts, nil
}

// Create parents as the platform user, not as root-owned Docker mount scaffolding.
func prepareMountParent(stateDir, parent string) error {
	relative, err := filepath.Rel(stateDir, parent)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("host mount parent must remain within task state")
	}
	current := stateDir
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if err := os.Mkdir(current, 0o700); err != nil && !os.IsExist(err) {
				return fmt.Errorf("create home mount parent: %w", err)
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return fmt.Errorf("inspect home mount parent: %w", err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("home mount parent must be a directory without symlinks")
		}
	}
	return nil
}
