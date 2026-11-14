package docker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/josemiguelmelo/orkest/internal/server/taskeditor"
	"github.com/moby/moby/api/types/container"
)

func TestEditorRequiresProtectedWorkerAncestry(t *testing.T) {
	work, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(work, "repositories")
	repo := filepath.Join(parent, "github", "repo")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	roots := []taskeditor.Root{{Source: repo}}
	for _, tc := range []struct {
		name    string
		change  func(*container.InspectResponse)
		allowed bool
	}{
		{name: "protected", allowed: true},
		{name: "legacy", change: func(c *container.InspectResponse) { delete(c.Config.Labels, checkoutMountLayoutLabel) }},
		{name: "writable parents", change: func(c *container.InspectResponse) { c.Mounts[1].RW = true }},
		{name: "privileged", change: func(c *container.InspectResponse) { c.HostConfig.Privileged = true }},
		{name: "mount capability", change: func(c *container.InspectResponse) { c.HostConfig.CapAdd = []string{"SYS_ADMIN"} }},
		{name: "writable alias", change: func(c *container.InspectResponse) {
			c.Mounts = append(c.Mounts, container.MountPoint{Source: work, Destination: "/mnt/host/alias", RW: true})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			worker := container.InspectResponse{Config: &container.Config{Labels: map[string]string{checkoutMountLayoutLabel: checkoutMountLayout}}, HostConfig: &container.HostConfig{}, Mounts: []container.MountPoint{{Source: work, Destination: workMountPath, RW: true}, {Source: parent, Destination: workMountPath + "/repositories"}, {Source: repo, Destination: workMountPath + "/repositories/github/repo", RW: true}}}
			if tc.change != nil {
				tc.change(&worker)
			}
			if err := protectedWorker(worker, work, roots); (err == nil) != tc.allowed {
				t.Fatalf("admission: %v", err)
			}
		})
	}
}

func TestProtectedCheckoutMountsRejectSymlinkParents(t *testing.T) {
	work := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(work, "repositories")); err != nil {
		t.Fatal(err)
	}
	if _, err := protectedCheckoutMounts(work, nil); err == nil {
		t.Fatal("symlinked parent accepted")
	}
}
