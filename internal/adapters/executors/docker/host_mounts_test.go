package docker

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	coreruntime "github.com/josemiguelmelo/orkest/internal/contracts/runtimecatalog"
	"github.com/josemiguelmelo/orkest/internal/platform/sessionpath"
	mountmodel "github.com/josemiguelmelo/orkest/internal/server/workspacemount/model"
)

func TestHostMountsValidatePathsAndPreserveAccessMode(t *testing.T) {
	for _, test := range []struct {
		name     string
		source   string
		readOnly bool
		wantErr  bool
	}{
		{"read only", "/srv/input", true, false},
		{"read write", "/srv/output", false, false},
		{"relative path", "relative/path", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := &fakeEngine{containerID: "container", hostPort: "49152", running: true}
			executor, err := NewWithClient(context.Background(), Config{DataRoot: t.TempDir()}, engine)
			if err != nil {
				t.Fatal(err)
			}
			config := runConfig()
			config.HostMounts = []mountmodel.Mount{{Source: test.source, Target: "/mnt/host/data", ReadOnly: test.readOnly}}
			_, err = executor.StartAgentctl(context.Background(), config)
			if test.wantErr {
				if err == nil || engine.created.Config != nil {
					t.Fatal("invalid mount created a container", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			mounts := engine.created.HostConfig.Mounts
			if len(mounts) != 3 || mounts[2].Source != test.source || mounts[2].Target != "/mnt/host/data" || mounts[2].ReadOnly != test.readOnly || mounts[2].Type != "bind" {
				t.Fatalf("unexpected mounts %+v", mounts)
			}
			config.RestoreSession = true
			if _, err := executor.StartAgentctl(context.Background(), config); err != nil {
				t.Fatal("saved mounts failed on replacement", err)
			}
		})
	}
}

func TestHomeTargetsResolveToRuntimeHomeAcrossReplacementAndRuntimes(t *testing.T) {
	for _, provider := range coreruntime.Providers() {
		t.Run(string(provider.Runtime()), func(t *testing.T) {
			engine := &fakeEngine{containerID: "container", hostPort: "49152", running: true}
			executor, err := NewWithClient(context.Background(), Config{DataRoot: t.TempDir()}, engine)
			if err != nil {
				t.Fatal(err)
			}
			config := runConfig()
			config.Runtime = provider.Runtime()
			config.Model.Provider = map[coreruntime.Runtime]string{
				coreruntime.OpenCode: "provider", coreruntime.ClaudeCode: "anthropic",
				coreruntime.Codex: "openai", coreruntime.GitHubCopilot: "github-copilot", coreruntime.Kiro: "kiro",
			}[config.Runtime]
			config.TaskSessionID = "session"
			config.HostMounts = []mountmodel.Mount{
				{Source: "/srv/aws", Target: "~/.aws", ReadOnly: true},
				{Source: "/srv/settings.xml", Target: "~/.m2/settings.xml", ReadOnly: true},
				{Source: "/srv/gradle.properties", Target: "~/.gradle/gradle.properties", ReadOnly: true},
			}
			saved := config.Clone()
			if _, err := executor.StartAgentctl(context.Background(), config); err != nil {
				t.Fatal(err)
			}
			home := sessionpath.Home(stateMountPath, config.OrganizationID, config.WorkspaceID, config.TaskID, config.TaskSessionID)
			mounts := engine.created.HostConfig.Mounts[2:]
			for i, relative := range []string{".aws", ".m2/settings.xml", ".gradle/gradle.properties"} {
				if mounts[i].Target != filepath.Join(home, relative) || mounts[i].Source != config.HostMounts[i].Source || !mounts[i].ReadOnly {
					t.Fatalf("home target did not resolve: %+v", mounts[i])
				}
			}
			_, stateDir, err := executor.taskDirectories(config)
			if err != nil {
				t.Fatal(err)
			}
			localHome := sessionpath.Home(stateDir, config.OrganizationID, config.WorkspaceID, config.TaskID, config.TaskSessionID)
			for _, relative := range []string{".", ".m2", ".gradle"} {
				info, err := os.Stat(filepath.Join(localHome, relative))
				if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
					t.Fatal("mount parents were not prepared as private directories", err)
				}
			}
			if !reflect.DeepEqual(config.HostMounts, saved.HostMounts) {
				t.Fatal("resolved paths mutated the persisted home-relative targets")
			}
			config.RunID, config.RestoreSession = "replacement", true
			if _, err := executor.StartAgentctl(context.Background(), config); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(engine.created.HostConfig.Mounts[2:], mounts) {
				t.Fatal("replacement changed credential paths")
			}
			config.TaskSessionID = "new-session"
			if _, err := executor.StartAgentctl(context.Background(), config); err != nil {
				t.Fatal(err)
			}
			if engine.created.HostConfig.Mounts[2].Target == mounts[0].Target {
				t.Fatal("new session reused the old runtime home")
			}
			config.TaskSessionID = ""
			if _, err := executor.StartAgentctl(context.Background(), config); err != nil {
				t.Fatal(err)
			}
			legacyHome := sessionpath.Home(stateMountPath, config.OrganizationID, config.WorkspaceID, config.TaskID, "")
			if engine.created.HostConfig.Mounts[2].Target != filepath.Join(legacyHome, ".aws") {
				t.Fatal("legacy home target did not resolve")
			}
		})
	}
}

func TestHomeMountTargetRejectsSymlinks(t *testing.T) {
	engine := &fakeEngine{containerID: "container", hostPort: "49152", running: true}
	executor, err := NewWithClient(context.Background(), Config{DataRoot: t.TempDir()}, engine)
	if err != nil {
		t.Fatal(err)
	}
	config := runConfig()
	config.HostMounts = []mountmodel.Mount{{Source: "/srv/aws", Target: "~/.aws", ReadOnly: true}}
	_, stateDir, err := executor.taskDirectories(config)
	if err != nil {
		t.Fatal(err)
	}
	home := sessionpath.Home(stateDir, config.OrganizationID, config.WorkspaceID, config.TaskID, config.TaskSessionID)
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(home, ".aws")); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.StartAgentctl(context.Background(), config); err == nil || engine.created.Config != nil {
		t.Fatal("home target symlink created a container", err)
	}
}

func TestHomeMountParentsRejectSymlinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "home")); err != nil {
		t.Fatal(err)
	}
	if err := prepareMountParent(root, filepath.Join(root, "home", ".m2")); err == nil {
		t.Fatal("home mount parent escaped through a symlink")
	}
	if _, err := os.Stat(filepath.Join(outside, ".m2")); !os.IsNotExist(err) {
		t.Fatal("symlink target was modified", err)
	}
	if err := prepareMountParent(root, filepath.Dir(root)); err == nil {
		t.Fatal("parent traversal accepted")
	}
}
